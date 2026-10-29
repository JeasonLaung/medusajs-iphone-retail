package cart

import (
	"context"
	"errors"
	"strconv"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// CompleteCurrentPaidOrder resolves the server's saved operation. Neither the
// operation/version nor payment/order identity is accepted from a return URL.
// The read is only routing: CompletePaidOrder reauthorizes all facts under the
// owning transaction lock, including closed-cart receipt replay and revocation.
func CompleteCurrentPaidOrder(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, o InitialPaymentOptions) (CompletionResult, error) {
	if db == nil || db.Statement == nil || !refOK(store, ref) {
		return CompletionResult{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return CompletionResult{}, ErrInput
	}
	var saved struct {
		OperationID  string
		InputVersion uint64
	}
	r := db.WithContext(ctx).Table("cart_ownership o").Select("a.operation_id,a.input_version").Joins("JOIN cart_checkout_attempts a ON a.cart_id=o.cart_id AND a.operation_id=o.checkout_attempt_id AND a.generation=o.generation").Where("o.cart_id=? AND o.store_id=? AND o.generation=? AND o.active=TRUE", ref.CartID, store, ref.Generation).Take(&saved)
	if errors.Is(r.Error, gorm.ErrRecordNotFound) {
		return CompletionResult{}, ErrDenied
	}
	if r.Error != nil {
		return CompletionResult{}, ErrStorage
	}
	return CompletePaidOrder(ctx, db, store, ref, InitialPaymentCommand{OperationID: saved.OperationID, Version: strconv.FormatUint(saved.InputVersion, 10)}, o)
}
