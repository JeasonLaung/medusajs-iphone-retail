package customer

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpdateProfile changes only fields supported by the original profile name
// and phone forms. Omitted fields remain unchanged. No email/ID/role binding.
func (s *Store) UpdateProfile(ctx context.Context, token string, fields map[string]string) (Account, error) {
	if len(fields) == 0 {
		return Account{}, ErrInvalid
	}
	for name, value := range fields {
		switch name {
		case "first_name", "last_name":
			if !cleanText(value, 160, true) {
				return Account{}, ErrInvalid
			}
		case "phone":
			if !cleanText(value, 64, false) {
				return Account{}, ErrInvalid
			}
		default:
			return Account{}, ErrInvalid
		}
	}
	if !tokenOK(token) {
		return Account{}, ErrUnauthorized
	}
	if !s.ready() {
		return Account{}, ErrUnavailable
	}
	principal, err := s.sessions.Resolve(ctx, sessionstore.Customer, token)
	if err != nil {
		return Account{}, customerSessionError(err)
	}
	var result Account
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current record
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", principal.UserID).Take(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUnauthorized
			}
			return ErrUnavailable
		}
		// Recheck Redis after waiting for the row lock; logout/revocation while
		// waiting cannot authorize a stale command from the first resolution.
		fresh, err := s.sessions.Resolve(ctx, sessionstore.Customer, token)
		if err != nil {
			return customerSessionError(err)
		}
		if fresh.UserID != current.ID || !current.Enabled || current.VerifiedAt == nil || current.AuthVersion == 0 || subtle.ConstantTimeCompare([]byte(fresh.Version), []byte(version(current))) != 1 {
			return ErrUnauthorized
		}
		changes := map[string]any{"updated_at": time.Now().UTC().Truncate(time.Microsecond)}
		for name, value := range fields {
			changes[name] = value
			switch name {
			case "first_name":
				current.FirstName = value
			case "last_name":
				current.LastName = value
			case "phone":
				current.Phone = value
			}
		}
		current.UpdatedAt = changes["updated_at"].(time.Time)
		if err := tx.Model(&record{}).Where("id=?", current.ID).Updates(changes).Error; err != nil {
			return ErrUnavailable
		}
		result = current.Account
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return Account{}, ErrUnauthorized
		}
		return Account{}, ErrUnavailable
	}
	return result, nil
}
func customerSessionError(err error) error {
	if errors.Is(err, sessionstore.ErrUnavailable) {
		return ErrUnavailable
	}
	return ErrUnauthorized
}
