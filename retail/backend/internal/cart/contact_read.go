package cart

import (
	"context"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// Only the current owned draft's copied contact values are exposed. Catalog
// and public cart snapshots continue to exclude all of these private fields.
type ContactDetail struct {
	CartID   string          `json:"cart_id"`
	Version  string          `json:"version"`
	Email    string          `json:"email"`
	Shipping *ContactAddress `json:"shipping_address"`
	Billing  *ContactAddress `json:"billing_address"`
}

func ReadContacts(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference) (ContactDetail, error) {
	var out ContactDetail
	err := readCartSnapshot(ctx, db, store, ref, func(tx *gorm.DB, cart Snapshot) error {
		var err error
		out, err = readContactValues(tx, cart)
		return err
	})
	if err != nil {
		return ContactDetail{}, err
	}
	return out, nil
}

func readContactValues(tx *gorm.DB, cart Snapshot) (ContactDetail, error) {
	var out ContactDetail
	var row struct{ Email, ShippingAddressID, BillingAddressID *string }
	if tx.Table("carts").Select("email,shipping_address_id,billing_address_id").Where("id=? AND deleted_at IS NULL", cart.ID).Take(&row).Error != nil {
		return ContactDetail{}, ErrSchema
	}
	out = ContactDetail{CartID: cart.ID, Version: cart.Version}
	if row.Email != nil {
		out.Email = *row.Email
	}
	if out.Email != "" {
		parsed, err := mail.ParseAddress(out.Email)
		if err != nil || parsed.Address != out.Email || parsed.Name != "" || len(out.Email) > 254 || !utf8.ValidString(out.Email) || strings.ContainsFunc(out.Email, func(r rune) bool { return r < 32 || r == 127 }) {
			return ContactDetail{}, ErrSchema
		}
	}
	for _, a := range []struct {
		id     *string
		target **ContactAddress
	}{{row.ShippingAddressID, &out.Shipping}, {row.BillingAddressID, &out.Billing}} {
		if a.id == nil {
			continue
		}
		if !idPattern.MatchString(*a.id) {
			return ContactDetail{}, ErrSchema
		}
		var address ContactAddress
		// Null optional fields become empty input values; never serialize the row.
		result := tx.Table("cart_addresses").Select("COALESCE(first_name,'') AS first_name,COALESCE(last_name,'') AS last_name,COALESCE(company,'') AS company,COALESCE(address_1,'') AS address1,COALESCE(address_2,'') AS address2,COALESCE(city,'') AS city,COALESCE(country_code,'') AS country,COALESCE(province,'') AS province,COALESCE(postal_code,'') AS postal_code,COALESCE(phone,'') AS phone").Where("id=? AND deleted_at IS NULL", *a.id).Scan(&address)
		if result.Error != nil || result.RowsAffected != 1 || !validContactAddress(address) {
			return ContactDetail{}, ErrSchema
		}
		*a.target = &address
	}
	return out, nil
}
