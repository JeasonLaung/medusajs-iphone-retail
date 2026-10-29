package customer

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAddressMissing = errors.New("customer address not found")

// Address contains only the fields used by the existing storefront forms.
// Ownership always comes from the authenticated account, never a request field.
type Address struct {
	ID                string    `json:"id"`
	FirstName         string    `json:"first_name"`
	LastName          string    `json:"last_name"`
	Company           string    `json:"company"`
	Address1          string    `json:"address_1" gorm:"column:address_1"`
	Address2          string    `json:"address_2" gorm:"column:address_2"`
	City              string    `json:"city"`
	PostalCode        string    `json:"postal_code"`
	Province          string    `json:"province"`
	CountryCode       string    `json:"country_code"`
	Phone             string    `json:"phone"`
	IsDefaultBilling  bool      `json:"is_default_billing"`
	IsDefaultShipping bool      `json:"is_default_shipping"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
type addressRecord struct {
	Address    `gorm:"embedded"`
	CustomerID string
}

func (addressRecord) TableName() string { return "customer_account_addresses" }

var addressText = map[string]struct {
	max      int
	required bool
}{
	"first_name": {160, true}, "last_name": {160, true}, "company": {255, false},
	"address_1": {255, true}, "address_2": {255, false}, "city": {160, true},
	"postal_code": {32, true}, "province": {160, false}, "phone": {64, false},
}

func validAddressID(id string) bool {
	if !strings.HasPrefix(id, "addr_") {
		return false
	}
	parsed, err := uuid.Parse(strings.TrimPrefix(id, "addr_"))
	return err == nil && id == "addr_"+parsed.String() && parsed != uuid.Nil
}
func addressFields(fields map[string]string, create bool) (map[string]any, error) {
	if len(fields) == 0 {
		return nil, ErrInvalid
	}
	out := map[string]any{}
	for key, value := range fields {
		if rule, ok := addressText[key]; ok {
			if !cleanText(value, rule.max, rule.required) {
				return nil, ErrInvalid
			}
			out[key] = value
			continue
		}
		switch key {
		case "country_code":
			if len(value) != 2 || value[0] < 'a' || value[0] > 'z' || value[1] < 'a' || value[1] > 'z' {
				return nil, ErrInvalid
			}
			out[key] = value
		case "is_default_billing", "is_default_shipping":
			if value != "true" && value != "false" {
				return nil, ErrInvalid
			}
			out[key] = value == "true"
		default:
			return nil, ErrInvalid
		}
	}
	if create {
		for key, rule := range addressText {
			if rule.required {
				if _, ok := out[key]; !ok {
					return nil, ErrInvalid
				}
			}
		}
		if _, ok := out["country_code"]; !ok {
			return nil, ErrInvalid
		}
	}
	return out, nil
}

// Account-row serialization also serializes default-address changes. A fresh
// Redis check after the lock rejects a command revoked while it was waiting.
func (s *Store) addressTransaction(ctx context.Context, token string, action func(*gorm.DB, string) error) error {
	if !tokenOK(token) {
		return ErrUnauthorized
	}
	if !s.ready() {
		return ErrUnavailable
	}
	principal, err := s.sessions.Resolve(ctx, sessionstore.Customer, token)
	if err != nil {
		return customerSessionError(err)
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var account record
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", principal.UserID).Take(&account).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUnauthorized
			}
			return err
		}
		fresh, err := s.sessions.Resolve(ctx, sessionstore.Customer, token)
		if err != nil {
			return customerSessionError(err)
		}
		if fresh.UserID != account.ID || !account.Enabled || account.VerifiedAt == nil || account.AuthVersion == 0 || subtle.ConstantTimeCompare([]byte(fresh.Version), []byte(version(account))) != 1 {
			return ErrUnauthorized
		}
		return action(tx, account.ID)
	})
	if err == nil || errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrAddressMissing) {
		return err
	}
	return ErrUnavailable
}
func (s *Store) Addresses(ctx context.Context, token string) ([]Address, error) {
	out := []Address{}
	err := s.addressTransaction(ctx, token, func(tx *gorm.DB, id string) error {
		rows := []addressRecord{}
		if err := tx.Where("customer_id=?", id).Order("created_at,id").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, row.Address)
		}
		return nil
	})
	return out, err
}

// An empty id creates; a nonempty id updates only an address owned by this
// account. Default switches and the requested write succeed or roll back together.
func (s *Store) SaveAddress(ctx context.Context, token, id string, fields map[string]string) (Address, error) {
	create := id == ""
	if !create && !validAddressID(id) {
		return Address{}, ErrAddressMissing
	}
	changes, err := addressFields(fields, create)
	if err != nil {
		return Address{}, err
	}
	var result Address
	err = s.addressTransaction(ctx, token, func(tx *gorm.DB, owner string) error {
		if !create {
			var existing addressRecord
			if err := tx.Where("customer_id=? AND id=?", owner, id).Take(&existing).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrAddressMissing
				}
				return err
			}
		}
		// Saving a contact address does not establish shipping eligibility.
		// The original country selector includes Europe outside configured regions;
		// checkout must resolve current regions and shipping rules separately.
		stamp := time.Now().UTC().Truncate(time.Microsecond)
		for _, key := range []string{"is_default_billing", "is_default_shipping"} {
			if changes[key] == true {
				if err := tx.Model(&addressRecord{}).Where("customer_id=? AND "+key+"=TRUE", owner).Updates(map[string]any{key: false, "updated_at": stamp}).Error; err != nil {
					return err
				}
			}
		}
		changes["updated_at"] = stamp
		if create {
			id = "addr_" + uuid.NewString()
			changes["id"], changes["customer_id"], changes["created_at"] = id, owner, stamp
			if err := tx.Model(&addressRecord{}).Create(changes).Error; err != nil {
				return err
			}
		} else if err := tx.Model(&addressRecord{}).Where("id=? AND customer_id=?", id, owner).Updates(changes).Error; err != nil {
			return err
		}
		var saved addressRecord
		if err := tx.Where("id=? AND customer_id=?", id, owner).Take(&saved).Error; err != nil {
			return err
		}
		result = saved.Address
		return nil
	})
	return result, err
}
func (s *Store) DeleteAddress(ctx context.Context, token, id string) error {
	if !validAddressID(id) {
		return ErrAddressMissing
	}
	return s.addressTransaction(ctx, token, func(tx *gorm.DB, owner string) error {
		result := tx.Where("id=? AND customer_id=?", id, owner).Delete(&addressRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAddressMissing
		}
		return nil
	})
}
