package customer

import (
	"context"
	"crypto/subtle"
	"errors"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrCartConflict = errors.New("customer cart transfer unavailable")

// TransferCart requires both the current new-account session and possession of
// the current cart. It never finds carts, customers or orders by email.
func (s *Store) TransferCart(ctx context.Context, token, possession, store string, in cart.CustomerTransferCommand, providers []string) (cart.ContactResult, error) {
	failed := cart.ContactResult{}
	if !tokenOK(token) || !tokenOK(possession) {
		return failed, ErrUnauthorized
	}
	if !s.ready() {
		return failed, ErrUnavailable
	}
	resolver, ok := s.sessions.(orderPossession)
	if !ok {
		return failed, ErrUnavailable
	}
	principal, err := s.sessions.Resolve(ctx, sessionstore.Customer, token)
	if err != nil {
		return failed, customerSessionError(err)
	}
	ref, err := resolver.ResolveCart(ctx, possession)
	if err != nil {
		return failed, customerSessionError(err)
	}
	if ref.StoreID != store {
		return failed, ErrUnauthorized
	}
	var authErr error
	result, err := cart.TransferCustomer(ctx, s.db, store, ref, in, providers, func(tx *gorm.DB) (string, string, error) {
		var account record
		r := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", principal.UserID).Take(&account)
		if r.Error != nil {
			authErr = ErrUnavailable
			if errors.Is(r.Error, gorm.ErrRecordNotFound) {
				authErr = ErrUnauthorized
			}
			return "", "", authErr
		}
		fresh, e := s.sessions.Resolve(ctx, sessionstore.Customer, token)
		if e != nil {
			authErr = customerSessionError(e)
			return "", "", authErr
		}
		current, e := resolver.ResolveCart(ctx, possession)
		if e != nil {
			authErr = customerSessionError(e)
			return "", "", authErr
		}
		if fresh.UserID != account.ID || current != ref || !account.Enabled || account.VerifiedAt == nil || account.AuthVersion == 0 || subtle.ConstantTimeCompare([]byte(fresh.Version), []byte(version(account))) != 1 {
			authErr = ErrUnauthorized
			return "", "", authErr
		}
		return account.ID, account.Email, nil
	})
	if authErr != nil {
		return failed, authErr
	}
	if errors.Is(err, cart.ErrInput) {
		return failed, ErrInvalid
	}
	if errors.Is(err, cart.ErrConflict) || errors.Is(err, cart.ErrDenied) || errors.Is(err, cart.ErrItemUnavailable) || errors.Is(err, cart.ErrItemStock) {
		return failed, ErrCartConflict
	}
	if err != nil {
		return failed, ErrUnavailable
	}
	return result, nil
}
