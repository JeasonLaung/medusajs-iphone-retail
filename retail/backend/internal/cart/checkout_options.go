package cart

import (
	"context"
	"database/sql"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaymentProvider struct {
	ID string `json:"id"`
}
type PaymentOptions struct {
	CartID         string            `json:"cart_id"`
	Version        string            `json:"version"`
	Providers      []PaymentProvider `json:"providers"`
	AttemptStarted bool              `json:"attempt_started"`
}

// A private projection of configured code AND current region bindings. An
// available method is not a quote, stock reservation or authorization to pay.
func ReadPaymentOptions(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, o InitialPaymentOptions) (PaymentOptions, error) {
	if db == nil || db.Statement == nil || !refOK(store, ref) {
		return PaymentOptions{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return PaymentOptions{}, ErrInput
	}
	if err := initialOptions(o); err != nil {
		return PaymentOptions{}, err
	}
	var out PaymentOptions
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := initialReady(ctx, tx); err != nil {
			return err
		}
		channel, err := storeChannel(ctx, tx, store)
		if err != nil {
			return err
		}
		current, err := lockCurrent(ctx, tx, store, channel, ref, "SHARE")
		if err != nil {
			return err
		}
		var owner struct{ CheckoutAttemptID *string }
		if tx.Table("cart_ownership").Select("checkout_attempt_id").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&owner).Error != nil {
			return ErrStorage
		}
		out = PaymentOptions{CartID: current.ID, Version: current.Version, Providers: []PaymentProvider{}, AttemptStarted: owner.CheckoutAttemptID != nil}
		if out.AttemptStarted {
			return nil
		}
		if err = initialProvider(tx, current); err == ErrConfiguration {
			return nil
		} else if err != nil {
			return err
		}
		out.Providers = append(out.Providers, PaymentProvider{ID: hostedMollie})
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: true})
	if err != nil {
		return PaymentOptions{}, sanitize(err)
	}
	return out, nil
}
func (h *HTTPHandler) PaymentOptions(c *gin.Context) { h.detail(c, "payment-options", false) }
