package cart

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/promotion"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// PromotionContext is private compute input. It includes contact/address and
// metadata facts; never expose it through an API, cache, receipt or log.
type PromotionContext struct {
	JSON            json.RawMessage
	CurrencyEpsilon string
}

// ReadPromotionContext joins the selected order-level fields to target facts
// in the caller-owned transaction. Current cart authority remains anonymous:
// customer-bound carts are rejected by LockForWrite, never adopted by email or
// historical actor ID. New customer authentication/association is separate.
// This prepares rules, not a public refresh command or payment authorization.
func ReadPromotionContext(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string) (PromotionContext, error) {
	failed := PromotionContext{}
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return failed, err
	}
	if promotion.Ready(ctx, tx) != nil || payments.Ready(ctx, tx) != nil {
		return failed, ErrSchema
	}
	amounts, err := assembleAmounts(tx, cart)
	if err != nil {
		return failed, err
	}
	targets, err := assemblePromotionTargets(tx, cart, amounts)
	if err != nil {
		return failed, err
	}
	rows, err := promotionRows(tx, "carts", "id", []string{cart.ID}, 1, promotionFields{text: "id email currency_code locale region_id sales_channel_id shipping_address_id customer_id", json: "metadata", date: "completed_at"})
	if err != nil || len(rows) != 1 {
		return failed, ErrSchema
	}
	out := rows[0]
	if out["id"] != cart.ID || out["currency_code"] != cart.CurrencyCode || out["region_id"] != cart.RegionID || out["sales_channel_id"] != cart.SalesChannelID || out["customer_id"] != nil || out["completed_at"] != nil {
		return failed, ErrConflict
	}
	locale, _ := out["locale"].(string)
	if locale != cart.Locale {
		return failed, ErrConflict
	}
	delete(out, "customer_id")
	out["customer"] = nil
	out["items"], out["shipping_methods"] = targets.Items, targets.ShippingMethods
	// Match the original graph's selected order amounts; do not accidentally
	// widen its rule surface by serializing the entire AmountDetail DTO.
	for _, key := range []string{"subtotal", "original_item_total", "item_total", "total", "original_item_subtotal", "item_subtotal", "shipping_subtotal"} {
		value, ok := amounts.Totals[key]
		if !ok {
			return failed, ErrSchema
		}
		out[key] = json.Number(value)
	}
	// The installed refresh field list requests quantity, but Cart.parse() and
	// decorateCartTotals do not provide a top-level quantity. Preserve missing
	// semantics rather than inventing a sum that could change order rules.
	regions, err := promotionRows(tx, "commerce_regions", "id", []string{cart.RegionID}, 1, promotionFields{text: "id name currency_code", boolean: "automatic_taxes", json: "metadata", date: "created_at updated_at deleted_at"})
	if err != nil || len(regions) != 1 || regions[0]["currency_code"] != cart.CurrencyCode {
		return failed, ErrSchema
	}
	out["region"] = regions[0]
	out["shipping_address"] = nil
	if id, ok := out["shipping_address_id"].(string); ok {
		addresses, err := promotionRows(tx, "cart_addresses", "id", []string{id}, 1, promotionFields{text: "id customer_id company first_name last_name address_1 address_2 city country_code province postal_code phone", json: "metadata", date: "created_at updated_at deleted_at"})
		if err != nil {
			return failed, err
		}
		if len(addresses) == 1 {
			out["shipping_address"] = addresses[0]
		}
	}
	delete(out, "shipping_address_id")
	links, err := promotionRows(tx, "cart_promotions", "cart_id", []string{cart.ID}, 1000, promotionFields{text: "id promotion_id"})
	if err != nil {
		return failed, err
	}
	ids := []string{}
	for _, link := range links {
		id, ok := link["promotion_id"].(string)
		if !ok || id == "" {
			return failed, ErrSchema
		}
		ids = append(ids, id)
	}
	promotions, err := promotionRows(tx, "promotions", "id", ids, 1000, promotionFields{text: "id code"})
	if err != nil {
		return failed, err
	}
	out["promotions"] = promotions
	// Only live link AND collection participate. The original graph's relation
	// is singular; multiple live collections must be resolved, not chosen.
	var collectionIDs []string
	if tx.Table("cart_payment_collections l").Select("p.id").Joins("JOIN payment_collections p ON p.id=l.payment_collection_id AND p.deleted_at IS NULL").Where("l.cart_id=? AND l.deleted_at IS NULL", cart.ID).Order("p.id").Limit(2).Scan(&collectionIDs).Error != nil || len(collectionIDs) > 1 {
		return failed, ErrSchema
	}
	out["payment_collection"] = nil
	if len(collectionIDs) == 1 {
		collections, err := promotionRows(tx, "payment_collections", "id", collectionIDs, 1, promotionFields{text: "id currency_code", number: "amount", json: "raw_amount"})
		if err != nil || len(collections) != 1 {
			return failed, ErrSchema
		}
		collection := collections[0]
		if err := promotionEffective(collection, "amount"); err != nil {
			return failed, err
		}
		sessions, err := promotionRows(tx, "payment_sessions", "payment_collection_id", collectionIDs, 100, promotionFields{text: "id"})
		if err != nil {
			return failed, err
		}
		collection["payment_sessions"] = sessions
		out["payment_collection"] = collection
	}
	var currency struct{ DecimalDigits int }
	if tx.Table("commerce_currencies").Select("decimal_digits").Where("code=? AND deleted_at IS NULL", cart.CurrencyCode).Take(&currency).Error != nil || currency.DecimalDigits < 0 || currency.DecimalDigits > 18 {
		return failed, ErrSchema
	}
	epsilon := "1"
	if currency.DecimalDigits > 0 {
		epsilon = "0." + strings.Repeat("0", currency.DecimalDigits-1) + "1"
	}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > 1<<20 {
		return failed, ErrSchema
	}
	return PromotionContext{JSON: encoded, CurrencyEpsilon: epsilon}, nil
}
