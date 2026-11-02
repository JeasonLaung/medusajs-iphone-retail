package cart

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CustomerTransferCommand struct {
	OperationID string `json:"operation_id"`
	Version     string `json:"version"`
}

// A deployed anonymous checkout need not install the account-transfer extension.
// Once installed, missing or corrupt account tables fail closed. Disabling new
// transfers never disables consumption of an already saved account binding.
func customerCartInstalled(tx *gorm.DB) (bool, error) {
	var n int64
	if tx.Table("schema_migrations").Where("version=?", "0095_customer_cart_transfer.sql").Count(&n).Error != nil || n > 1 {
		return false, ErrSchema
	}
	return n == 1, nil
}

type cartCustomer struct{ CustomerID, StoreID, Generation string }

func readCartCustomer(tx *gorm.DB, ref sessionstore.CartReference) (*cartCustomer, error) {
	installed, err := customerCartInstalled(tx)
	if err != nil || !installed {
		return nil, err
	}
	var owner cartCustomer
	r := tx.Table("customer_cart_ownership").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&owner)
	if errors.Is(r.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if r.Error != nil {
		return nil, ErrStorage
	}
	if owner.CustomerID == "" || owner.StoreID != ref.StoreID || owner.Generation != ref.Generation {
		return nil, ErrConflict
	}
	return &owner, nil
}

// TransferCustomer attaches only the currently possessed cart. authenticate is a
// trusted, server-side account/session check, executed AFTER the cart row lock;
// it must lock the verified new account and recheck both capabilities. No account
// ID, email, store or generation from a browser is accepted as ownership proof.
func TransferCustomer(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in CustomerTransferCommand, providers []string, authenticate func(*gorm.DB) (string, string, error)) (ContactResult, error) {
	failed := ContactResult{}
	op, e := uuid.Parse(in.OperationID)
	version, v := strconv.ParseUint(in.Version, 10, 64)
	if e != nil || op.Version() != 4 || op.String() != in.OperationID || v != nil || version == 0 || version == ^uint64(0) || strconv.FormatUint(version, 10) != in.Version || !refOK(store, ref) || db == nil || db.Statement == nil || authenticate == nil || len(providers) > 100 {
		return failed, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return failed, ErrInput
	}
	for _, p := range providers {
		if !idPattern.MatchString(p) {
			return failed, ErrInput
		}
	}
	var result ContactResult
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := commandReady(ctx, tx, "cart.access"); err != nil {
			return err
		}
		var gate struct {
			Version int
			Enabled bool
		}
		r := tx.Raw("SELECT version,enabled FROM customer_cart_runtime WHERE id=1 FOR SHARE").Scan(&gate)
		if r.Error != nil || r.RowsAffected != 1 || gate.Version != 1 || !gate.Enabled {
			return ErrSchema
		}
		installed, err := customerCartInstalled(tx)
		if err != nil || !installed {
			return ErrSchema
		}
		channel, err := storeChannel(ctx, tx, store)
		if err != nil {
			return err
		}
		current, err := lockCurrent(ctx, tx, store, channel, ref, "UPDATE")
		if err != nil {
			return err
		}
		account, email, err := authenticate(tx)
		if err != nil {
			return err
		}
		parsed, e := mail.ParseAddress(email)
		if account == "" || len(account) > 80 || e != nil || parsed.Address != email || parsed.Name != "" || len(email) > 254 {
			return ErrDenied
		}
		owner, err := readCartCustomer(tx, ref)
		if err != nil {
			return err
		}
		if owner != nil && owner.CustomerID != account {
			return ErrDenied
		}
		var receipt struct {
			CustomerID, Generation      string
			InputVersion, ResultVersion uint64
		}
		r = tx.Table("customer_cart_operations").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&receipt)
		if r.Error == nil {
			nowVersion, _ := strconv.ParseUint(current.Version, 10, 64)
			if owner == nil || receipt.CustomerID != account || receipt.Generation != ref.Generation || receipt.InputVersion != version {
				return ErrConflict
			}
			if receipt.ResultVersion < version || receipt.ResultVersion > nowVersion {
				return ErrStorage
			}
			result = ContactResult{ref.CartID, strconv.FormatUint(receipt.ResultVersion, 10), true}
			return nil
		}
		if !errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return ErrStorage
		}
		if current.Version != in.Version {
			return ErrConflict
		}
		next := version
		if owner == nil {
			if _, err := reusablePaymentCollection(ctx, tx, current.ID); err != nil {
				return err
			}

			if err := supportedSavedShipping(tx, current.ID, providers); err != nil {
				return err
			}
			if tx.Exec("INSERT INTO customer_cart_ownership(cart_id,customer_id,store_id,generation,created_at) VALUES(?,?,?,?,UTC_TIMESTAMP(6))", ref.CartID, account, store, ref.Generation).Error != nil {
				return ErrStorage
			}
			if r := tx.Exec("UPDATE carts SET email=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND deleted_at IS NULL", email, current.ID); r.Error != nil {
				return ErrStorage
			}
			at := time.Now().UTC()
			if _, err := RefreshItemSnapshots(ctx, tx, store, ref, in.Version); err != nil {
				return err
			}
			if _, err := RefreshItemPrices(ctx, tx, store, ref, in.Version, at); err != nil {
				return err
			}
			if _, err := RefreshManualShippingMethods(ctx, tx, store, ref, in.Version, providers, at); err != nil {
				return err
			}
			// Medusa force_refresh selects updateTaxLines, whose separate
			// force_tax_calculation remains unset. Without a shipping country it
			// clears old lines; it must not force a country-less calculation.
			if _, err := RefreshSystemTaxes(ctx, tx, store, ref, in.Version, false); err != nil {
				return err
			}
			if _, err := RefreshPromotions(ctx, tx, store, ref, in.Version, PromotionRefresh{At: at}); err != nil {
				return err
			}
			if _, err := assembleAmounts(tx, current); err != nil {
				return err
			}
			if err := syncEmptyPaymentCollection(ctx, tx, store, ref, in.Version); err != nil {
				return err
			}
			if r := tx.Exec("UPDATE cart_ownership SET version=version+1,updated_at=UTC_TIMESTAMP(6) WHERE cart_id=? AND version=?", current.ID, version); r.Error != nil || r.RowsAffected != 1 {
				return ErrConflict
			}
			next++
		}
		// The original workflow skips all updates for the same customer. Preserve a
		// separately edited checkout email and amounts, while recording retry identity.
		if tx.Exec("INSERT INTO customer_cart_operations(cart_id,operation_id,customer_id,generation,input_version,result_version,created_at) VALUES(?,?,?,?,?,?,UTC_TIMESTAMP(6))", ref.CartID, in.OperationID, account, ref.Generation, version, next).Error != nil {
			return ErrStorage
		}
		result = ContactResult{ref.CartID, strconv.FormatUint(next, 10), false}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return failed, err
	}
	return result, nil
}

func attachCompletedCartCustomer(tx *gorm.DB, ref sessionstore.CartReference, order string) error {
	owner, err := readCartCustomer(tx, ref)
	if err != nil || owner == nil {
		return err
	}
	if tx.Exec("INSERT INTO customer_order_ownership(order_id,customer_id,cart_id,store_id,created_at) VALUES(?,?,?,?,UTC_TIMESTAMP(6))", order, owner.CustomerID, ref.CartID, ref.StoreID).Error != nil {
		return ErrStorage
	}
	return nil
}
