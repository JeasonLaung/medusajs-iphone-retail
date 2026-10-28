package cart

import (
	"context"
	"time"

	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/pricing"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ShippingQuotesGroup keeps uncovered profiles explicit. These private quotes
// have not passed warehouse inventory checks and cannot authorize checkout.
type ShippingQuotesGroup struct {
	Group  ShippingGroup
	Quotes []fulfillment.ManualFlatQuote
}

// ReadManualShippingQuotes binds the manual fixed-price phase to current cart
// authority and persisted address/channel/region/currency. Provider IDs must
// come from server registration. No browser context or provider data is taken.
// Custom option hooks, total-based price contexts and calculated providers are
// separate phases; an absent context must not be fabricated from address data.
// Caller owns the transaction, rollback, inventory checks and final command.
func ReadManualShippingQuotes(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string, manualProviderIDs []string, at time.Time) ([]ShippingQuotesGroup, error) {
	if at.IsZero() {
		return nil, ErrInput
	}
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return nil, err
	}
	groups, err := ReadShippingGroups(ctx, tx, store, ref, version)
	if err != nil {
		return nil, err
	}
	current := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Session(&gorm.Session{})
	var saved struct{ ShippingAddressID *string }
	r := current.Table("carts").Select("shipping_address_id").Where("id=? AND deleted_at IS NULL", cart.ID).Take(&saved)
	if r.Error != nil || r.RowsAffected != 1 {
		return nil, ErrSchema
	}
	selection := fulfillment.OptionSelection{Channel: cart.SalesChannelID}
	if saved.ShippingAddressID != nil {
		if !idPattern.MatchString(*saved.ShippingAddressID) {
			return nil, ErrSchema
		}
		var address struct{ CountryCode, Province, City, PostalCode string }
		r = current.Table("cart_addresses").Select("LEFT(country_code,4097) AS country_code,LEFT(province,4097) AS province,LEFT(city,4097) AS city,LEFT(postal_code,4097) AS postal_code").Where("id=? AND deleted_at IS NULL", *saved.ShippingAddressID).Take(&address)
		if r.Error != nil || r.RowsAffected != 1 {
			return nil, ErrSchema
		}
		selection.Address = fulfillment.ShippingAddress{Country: address.CountryCode, Province: address.Province, City: address.City, Postal: address.PostalCode}
	}
	profiles := []string{}
	for _, group := range groups {
		profiles = append(profiles, group.ProfileID)
	}
	quotes, err := fulfillment.ReadManualFlatQuotes(current, selection, profiles, manualProviderIDs, pricing.Context{Channel: cart.SalesChannelID, Region: cart.RegionID, Currency: cart.CurrencyCode}, at.UTC())
	if err != nil {
		return nil, err
	}
	byProfile := map[string][]fulfillment.ManualFlatQuote{}
	for _, quote := range quotes {
		byProfile[*quote.Option.ShippingProfileID] = append(byProfile[*quote.Option.ShippingProfileID], quote)
	}
	out := []ShippingQuotesGroup{}
	for _, group := range groups {
		matched := byProfile[group.ProfileID]
		if matched == nil {
			matched = []fulfillment.ManualFlatQuote{}
		}
		out = append(out, ShippingQuotesGroup{Group: group, Quotes: matched})
	}
	return out, nil
}
