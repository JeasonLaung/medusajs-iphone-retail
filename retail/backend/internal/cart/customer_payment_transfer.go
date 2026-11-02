package cart

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/mail"
	"strconv"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Server-only capability. No customer identity or email is accepted by cart
// HTTP. Every transaction must reauthorize both current capabilities and account.
type customerPaymentScopeKey struct{}
type customerPaymentScope struct {
	account, email string
	authenticate   func(*gorm.DB) (string, string, error)
}

func contextRequestDigest(ctx context.Context, in ContextCommand) [32]byte {
	if scope, ok := ctx.Value(customerPaymentScopeKey{}).(customerPaymentScope); ok {
		raw, _ := json.Marshal(struct {
			Context     ContextCommand
			Customer    string
			EmailSHA256 [32]byte
		}{in, scope.account, sha256.Sum256([]byte(scope.email))})
		return sha256.Sum256(raw)
	}
	raw, _ := json.Marshal(in)
	return sha256.Sum256(raw)
}

func validateCustomerPaymentScope(ctx context.Context, tx *gorm.DB, ref sessionstore.CartReference) error {
	scope, ok := ctx.Value(customerPaymentScopeKey{}).(customerPaymentScope)
	if !ok {
		return nil
	}
	var gate struct {
		Version int
		Enabled bool
	}
	r := tx.Raw("SELECT version,enabled FROM customer_cart_runtime WHERE id=1 FOR SHARE").Scan(&gate)
	if r.Error != nil || r.RowsAffected != 1 || gate.Version != 1 || !gate.Enabled {
		return ErrSchema
	}
	account, email, err := scope.authenticate(tx)
	if err != nil {
		return err
	}
	if account != scope.account || email != scope.email {
		return ErrDenied
	}
	owner, err := readCartCustomer(tx, ref)
	if err != nil {
		return err
	}
	if owner != nil && owner.CustomerID != account {
		return ErrDenied
	}
	return nil
}

// Called only in a scratch savepoint (preview) or the final owning transaction.
// This makes customer-qualified rules see the actual target before repricing.
func applyCustomerPaymentTarget(ctx context.Context, tx *gorm.DB, ref sessionstore.CartReference) error {
	scope, ok := ctx.Value(customerPaymentScopeKey{}).(customerPaymentScope)
	if !ok {
		return nil
	}
	if err := validateCustomerPaymentScope(ctx, tx, ref); err != nil {
		return err
	}
	owner, err := readCartCustomer(tx, ref)
	if err != nil {
		return err
	}
	if owner != nil {
		return ErrConflict
	}
	if tx.Exec("INSERT INTO customer_cart_ownership(cart_id,customer_id,store_id,generation,created_at) VALUES(?,?,?,?,UTC_TIMESTAMP(6))", ref.CartID, scope.account, ref.StoreID, ref.Generation).Error != nil {
		return ErrStorage
	}
	r := tx.Exec("UPDATE carts SET email=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND deleted_at IS NULL", scope.email, ref.CartID)
	if r.Error != nil || r.RowsAffected != 1 {
		return ErrStorage
	}
	return nil
}

func saveCustomerPaymentReceipt(ctx context.Context, tx *gorm.DB, ref sessionstore.CartReference, in ContextCommand) error {
	scope, ok := ctx.Value(customerPaymentScopeKey{}).(customerPaymentScope)
	if !ok {
		return nil
	}
	version, _ := strconv.ParseUint(in.Version, 10, 64)
	if tx.Exec("INSERT INTO customer_cart_operations(cart_id,operation_id,customer_id,generation,input_version,result_version,created_at) VALUES(?,?,?,?,?,?,UTC_TIMESTAMP(6))", ref.CartID, in.OperationID, scope.account, ref.Generation, version, version+1).Error != nil {
		return ErrStorage
	}
	return nil
}

// ContinueCustomerPaymentTransfer reuses the durable payment effect/compensation
// coordinator with a distinct authenticated target digest. Provider calls remain
// outside SQL; target ownership is saved only with final repricing and receipt.
func ContinueCustomerPaymentTransfer(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in CustomerTransferCommand, options InitialPaymentOptions, deletes *payments.DeleteAdapters, authenticate func(*gorm.DB) (string, string, error), restore bool) (ContextPaymentProgress, error) {
	if db == nil || db.Statement == nil || authenticate == nil || !refOK(store, ref) {
		return ContextPaymentProgress{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return ContextPaymentProgress{}, ErrInput
	}
	var command ContextCommand
	var scope customerPaymentScope
	var saved bool
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		channel, err := storeChannel(ctx, tx, store)
		if err != nil {
			return err
		}
		var owner struct{ CheckoutAttemptID, PaymentRefreshOperationID *string }
		if tx.Table("cart_ownership").Select("checkout_attempt_id,payment_refresh_operation_id").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&owner).Error != nil {
			return ErrDenied
		}
		checkout, operation := "", ""
		if owner.CheckoutAttemptID != nil {
			checkout = *owner.CheckoutAttemptID
		}
		if owner.PaymentRefreshOperationID != nil {
			operation = in.OperationID
		}
		current, err := lockCurrentAuthority(ctx, tx, store, channel, ref, "UPDATE", operation, checkout)
		if err != nil {
			return err
		}
		account, email, err := authenticate(tx)
		if err != nil {
			return err
		}
		parsed, err := mail.ParseAddress(email)
		if account == "" || len(account) > 80 || err != nil || parsed.Address != email || parsed.Name != "" || len(email) > 254 {
			return ErrDenied
		}
		scope = customerPaymentScope{account, email, authenticate}
		command = ContextCommand{OperationID: in.OperationID, Version: in.Version, RegionID: current.RegionID, Locale: current.Locale}
		// A later cart edit must not rewrite a completed operation's target.
		var count int64
		if tx.Table("cart_payment_refresh_operations").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID).Count(&count).Error != nil {
			return ErrStorage
		}
		if count == 1 {
			_, manifest, err := readContextPaymentOperation(tx, ref, in.OperationID)
			if err != nil {
				return err
			}
			command.RegionID, command.Locale = manifest.Target.RegionID, manifest.Target.Locale
		}
		if restore {
			if tx.Table("customer_cart_operations").Where("cart_id=? AND operation_id=? AND customer_id=? AND generation=? AND input_version=?", ref.CartID, in.OperationID, account, ref.Generation, in.Version).Count(&count).Error != nil {
				return ErrStorage
			}
			saved = count == 1
		}
		_, err = validateContext(command)
		return err
	})
	if err != nil {
		return ContextPaymentProgress{}, err
	}
	ctx = context.WithValue(ctx, customerPaymentScopeKey{}, scope)
	// A lost success response can lead the browser to ask for restoration.
	// Return the authenticated committed result instead of attempting to undo it.
	if restore && !saved {
		return ContinueContextPaymentRestore(ctx, db, store, ref, command, options)
	}
	return ContinueContextPayment(ctx, db, store, ref, command, options, deletes)
}
