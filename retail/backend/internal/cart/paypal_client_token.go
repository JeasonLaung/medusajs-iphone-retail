package cart

import (
	"context"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Token minting is not a financial operation and cannot alter a frozen attempt.
// It requires the existing card attempt and current cart authority. No caller
// supplied provider, credential, amount or card data is accepted.
func IssuePayPalClientToken(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, o InitialPaymentOptions) (payments.PayPalClientToken, error) {
	if db == nil || initialOptions(o) != nil {
		return payments.PayPalClientToken{}, ErrConfiguration
	}
	var issuer interface {
		ClientToken(context.Context) (payments.PayPalClientToken, error)
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		in, e := currentInitialCommand(ctx, tx, store, ref, o.Ring)
		if e != nil {
			return e
		}
		if in.ProviderID != "pp_paypal_card_paypal_card" {
			return ErrConfiguration
		}
		var row initialAttempt
		if tx.Table("cart_checkout_attempts").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&row).Error != nil {
			return ErrStorage
		}
		if !row.Activated || row.NeedsReview {
			return ErrConflict
		}
		a, e := o.Adapters.Lookup(in.ProviderID)
		if e != nil {
			return ErrConfiguration
		}
		var ok bool
		issuer, ok = a.(interface {
			ClientToken(context.Context) (payments.PayPalClientToken, error)
		})
		if !ok {
			return ErrConfiguration
		}
		return nil
	})
	if err != nil {
		return payments.PayPalClientToken{}, sanitize(err)
	}
	token, err := issuer.ClientToken(ctx)
	if err != nil {
		return payments.PayPalClientToken{}, ErrConfiguration
	}
	return token, nil
}
