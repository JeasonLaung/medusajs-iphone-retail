package cart

import (
	"context"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ShippingRefreshResult struct{ Updated, Removed int }

// RefreshManualShippingMethods refreshes saved method prices and removes
// ineligible/unpriced/orphan methods without changing method IDs or data.
// This is one phase of the caller-owned refresh transaction, not a public
// command: rollback on any error and complete taxes/promotions/payment/version
// and durable receipt before commit. Stock is not reserved by this phase.
func RefreshManualShippingMethods(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string, manualProviderIDs []string, at time.Time) (ShippingRefreshResult, error) {
	if at.IsZero() {
		return ShippingRefreshResult{}, ErrInput
	}
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return ShippingRefreshResult{}, err
	}
	var methods []struct {
		ID       string
		OptionID *string
	}
	if tx.WithContext(ctx).Table("cart_shipping_methods").Select("id,shipping_option_id AS option_id").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(101).Scan(&methods).Error != nil || len(methods) > 100 {
		return ShippingRefreshResult{}, ErrSchema
	}
	result := ShippingRefreshResult{}
	if len(methods) == 0 {
		return result, nil
	}
	singletonProfile := ""
	if len(methods) == 1 && methods[0].OptionID != nil {
		var profile []string
		if tx.Table("fulfillment_shipping_options o").Select("p.id").Joins("JOIN fulfillment_shipping_profiles p ON p.id=o.shipping_profile_id AND p.deleted_at IS NULL").Where("o.id=? AND o.deleted_at IS NULL", *methods[0].OptionID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Scan(&profile).Error != nil {
			return result, ErrSchema
		}
		if len(profile) == 1 {
			singletonProfile = profile[0]
		}
	}
	groups, err := readManualShippingQuotes(ctx, tx, store, ref, version, manualProviderIDs, at, singletonProfile)
	if err != nil {
		return result, err
	}
	quotes := map[string]fulfillment.ManualFlatQuote{}
	for _, group := range groups {
		for _, quote := range group.Quotes {
			quotes[quote.Option.ID] = quote
		}
	}
	type prepared struct {
		id, name, scalar, raw string
		inclusive             bool
	}
	updates := []prepared{}
	remove := []string{}
	for _, method := range methods {
		if !idPattern.MatchString(method.ID) {
			return ShippingRefreshResult{}, ErrSchema
		}
		var quote fulfillment.ManualFlatQuote
		ok := false
		if method.OptionID != nil {
			quote, ok = quotes[*method.OptionID]
		}
		if !ok {
			remove = append(remove, method.ID)
			continue
		}
		if !utf8.ValidString(quote.Option.Name) || len(quote.Option.Name) > 65536 {
			return ShippingRefreshResult{}, ErrSchema
		}
		scalar, raw, err := sqlAmount(quote.Price.Amount)
		if err != nil {
			return ShippingRefreshResult{}, err
		}
		updates = append(updates, prepared{id: method.ID, name: quote.Option.Name, scalar: scalar, raw: raw, inclusive: quote.Price.CalculatedTaxInclusive})
	}
	if len(remove) > 0 {
		r := tx.Table("cart_shipping_methods").Where("cart_id=? AND id IN ? AND deleted_at IS NULL", cart.ID, remove).Updates(map[string]any{"deleted_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
		if r.Error != nil || r.RowsAffected != int64(len(remove)) {
			return ShippingRefreshResult{}, ErrStorage
		}
	}
	for _, method := range updates {
		r := tx.Table("cart_shipping_methods").Where("id=? AND cart_id=? AND deleted_at IS NULL", method.id, cart.ID).Updates(map[string]any{"name": method.name, "amount": method.scalar, "raw_amount": method.raw, "is_tax_inclusive": method.inclusive, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
		if r.Error != nil || r.RowsAffected != 1 {
			return ShippingRefreshResult{}, ErrStorage
		}
	}
	result.Updated, result.Removed = len(updates), len(remove)
	return result, nil
}
