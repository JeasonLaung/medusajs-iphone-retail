package cart

import (
	"github.com/eurorefurb/retail/internal/customergroups"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// Cart possession is checked by the caller. Group identity comes solely from
// the explicit new-account binding, never legacy carts.customer_id or email.
func pricingCustomerGroups(tx *gorm.DB, ref sessionstore.CartReference) ([]string, error) {
	owner, e := readCartCustomer(tx, ref)
	if e != nil {
		return nil, e
	}
	if owner == nil {
		return []string{}, nil
	}
	groups, e := customergroups.ForAccountTx(tx, owner.CustomerID)
	if e != nil {
		return nil, ErrSchema
	}
	return groups, nil
}
