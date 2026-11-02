package cart

import (
	"context"
	"database/sql"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
)

type PaymentProvider struct {
	ID     string                    `json:"id"`
	PayPal *payments.PayPalSDKConfig `json:"paypal,omitempty"`
}
type PaymentOptions struct {
	CartID         string            `json:"cart_id"`
	Version        string            `json:"version"`
	Providers      []PaymentProvider `json:"providers"`
	AttemptStarted bool              `json:"attempt_started"`
	ActiveProvider *PaymentProvider  `json:"active_provider,omitempty"`
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
			var row initialAttempt
			if tx.Table("cart_checkout_attempts").Where("cart_id=? AND operation_id=?", ref.CartID, *owner.CheckoutAttemptID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&row).Error != nil {
				return ErrStorage
			}
			in := InitialPaymentCommand{OperationID: *owner.CheckoutAttemptID, Version: strconv.FormatUint(row.InputVersion, 10)}
			request, e := payments.ReadSessionRequest(ctx, tx, initialKey(ref, in, row), o.Ring)
			if e != nil {
				return e
			}
			defer clear(request.Data)
			defer clear(request.Context)
			if request.ProviderID == "pp_paypal_paypal" || request.ProviderID == "pp_paypal_card_paypal_card" {
				p, e := sdkPaymentProvider(o, request.ProviderID, request.Currency)
				if e != nil {
					return e
				}
				out.ActiveProvider = &p
			}
			return nil
		}
		for _, id := range []string{hostedMollie, "pp_paypal_paypal", "pp_paypal_card_paypal_card"} {
			if _, e := o.Adapters.Lookup(id); e != nil {
				continue
			}
			if e := initialProviderID(tx, current, id); e == ErrConfiguration {
				continue
			} else if e != nil {
				return e
			}
			p, e := sdkPaymentProvider(o, id, current.CurrencyCode)
			if e != nil {
				if id != hostedMollie {
					continue // Unsupported wallet currency must not hide hosted Mollie.
				}
				return e
			}
			out.Providers = append(out.Providers, p)
		}
		return nil
		// Protected request reads take a current receipt lock. MySQL rejects that
		// lock in READ ONLY transactions; this handler still performs no mutations.
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return PaymentOptions{}, sanitize(err)
	}
	return out, nil
}

func sdkPaymentProvider(o InitialPaymentOptions, id, currency string) (PaymentProvider, error) {
	p := PaymentProvider{ID: id}
	if id == hostedMollie {
		return p, nil
	}
	a, e := o.Adapters.Lookup(id)
	if e != nil {
		return p, ErrConfiguration
	}
	sdk, ok := a.(interface {
		SDKConfiguration(string) (payments.PayPalSDKConfig, error)
	})
	if !ok {
		return p, ErrConfiguration
	}
	c, e := sdk.SDKConfiguration(currency)
	if e != nil {
		return p, ErrConfiguration
	}
	p.PayPal = &c
	return p, nil
}
func (h *HTTPHandler) PaymentOptions(c *gin.Context) { h.detail(c, "payment-options", false) }
