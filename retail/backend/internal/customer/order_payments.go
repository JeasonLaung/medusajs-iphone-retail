package customer

import (
	"context"
	"errors"

	"github.com/eurorefurb/retail/internal/orderpayments"
	"gorm.io/gorm"
)

// The account lock and second Redis lookup are held through the monetary
// participant. Caller JSON never supplies account ID or authentication version.
func (s *Store) paymentTransaction(ctx context.Context, token string, action func(*gorm.DB, orderpayments.Principal) error) error {
	var domain error
	err := s.addressTransaction(ctx, token, func(tx *gorm.DB, id string) error {
		var account record
		if e := tx.Select("id,auth_version").Where("id=?", id).Take(&account).Error; e != nil {
			return e
		}
		domain = action(tx, orderpayments.Principal{AccountID: id, AuthVersion: account.AuthVersion})
		return domain
	})
	// addressTransaction hides SQL errors. Preserve only known participant errors;
	// a later commit failure must still report unavailable.
	if err != nil && domain != nil {
		switch {
		case errors.Is(domain, orderpayments.ErrDenied):
			return ErrOrderMissing
		case errors.Is(domain, orderpayments.ErrInput):
			return ErrInvalid
		case errors.Is(domain, orderpayments.ErrConflict):
			return ErrPaymentConflict
		}
	}
	return err
}

var ErrPaymentConflict = errors.New("order payment state changed")

func (s *Store) PrepareOrderPayment(ctx context.Context, token string, service *orderpayments.Service, in orderpayments.Input) (orderpayments.Result, error) {
	var out orderpayments.Result
	if service == nil {
		return out, ErrUnavailable
	}
	err := s.paymentTransaction(ctx, token, func(tx *gorm.DB, p orderpayments.Principal) error {
		var e error
		out, e = service.PrepareTx(ctx, tx, p, in)
		return e
	})
	if err != nil {
		return orderpayments.Result{}, err
	}
	return out, nil
}
func (s *Store) ReadOrderPayment(ctx context.Context, token string, service *orderpayments.Service, order, id string) (orderpayments.Result, error) {
	var out orderpayments.Result
	if service == nil {
		return out, ErrUnavailable
	}
	err := s.paymentTransaction(ctx, token, func(tx *gorm.DB, p orderpayments.Principal) error {
		var e error
		out, e = service.ReadTx(ctx, tx, p, order, id)
		return e
	})
	if err != nil {
		return orderpayments.Result{}, err
	}
	return out, nil
}
func (s *Store) CancelOrderPayment(ctx context.Context, token string, service *orderpayments.Service, order string, in orderpayments.CancelInput) (orderpayments.Result, error) {
	var out orderpayments.Result
	if service == nil {
		return out, ErrUnavailable
	}
	err := s.paymentTransaction(ctx, token, func(tx *gorm.DB, p orderpayments.Principal) error {
		var e error
		out, e = service.CancelTx(ctx, tx, p, order, in)
		return e
	})
	if err != nil {
		return orderpayments.Result{}, err
	}
	return out, nil
}
