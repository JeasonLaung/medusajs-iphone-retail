package cart

import (
	"context"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/internal/tax"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TaxRefreshResult struct {
	ItemLines, ShippingLines int
}

// RefreshSystemTaxes participates in a caller-owned cart write transaction.
// It rechecks durable authority/version, reads persisted region/address/items,
// and replaces the cart's tax lines using the configured built-in provider.
// The caller MUST roll back the WHOLE transaction after ANY error, and must
// provide Origin/CSRF, durable idempotency, version increment and all other
// refresh steps before committing. This is not a standalone command or HTTP
// capability. It neither changes tax configuration nor authorizes checkout.
// Translation/custom tax hooks and non-system providers remain unimplemented.
func RefreshSystemTaxes(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string, force bool) (TaxRefreshResult, error) {
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return TaxRefreshResult{}, err
	}
	var persisted struct {
		ShippingAddressID *string
		AutomaticTaxes    bool
	}
	r := tx.Raw(`SELECT c.shipping_address_id,r.automatic_taxes FROM carts c JOIN commerce_regions r ON r.id=c.region_id AND r.deleted_at IS NULL WHERE c.id=? AND c.deleted_at IS NULL FOR SHARE`, cart.ID).Scan(&persisted)
	if r.Error != nil || r.RowsAffected != 1 {
		return TaxRefreshResult{}, ErrSchema
	}
	context := tax.CalculationContext{Automatic: persisted.AutomaticTaxes, Force: force}
	if persisted.ShippingAddressID != nil {
		var address struct{ CountryCode, Province string }
		r := tx.Table("cart_addresses").Select("LEFT(country_code,3) AS country_code,LEFT(province,192) AS province").Where("id=? AND deleted_at IS NULL", *persisted.ShippingAddressID).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&address)
		if r.Error != nil || r.RowsAffected != 1 {
			return TaxRefreshResult{}, ErrSchema
		}
		context.Country, context.Province = address.CountryCode, address.Province
	}
	var products []struct {
		ID, ProductID, ProductTypeID *string
		IsGiftcard                   bool
	}
	if tx.Table("cart_line_items").Select("id,product_id,product_type_id,is_giftcard").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("id").Clauses(clause.Locking{Strength: "SHARE"}).Limit(1001).Scan(&products).Error != nil || len(products) > 1000 {
		return TaxRefreshResult{}, ErrSchema
	}
	items := make([]tax.CalculationItem, 0, len(products))
	for _, row := range products {
		if row.ID == nil || !idPattern.MatchString(*row.ID) {
			return TaxRefreshResult{}, ErrSchema
		}
		items = append(items, tax.CalculationItem{Item: tax.Item{ID: *row.ID, ProductID: row.ProductID, ProductTypeID: row.ProductTypeID}, Giftcard: row.IsGiftcard})
	}
	var shipping []struct {
		ID               string
		ShippingOptionID *string
	}
	if tx.Table("cart_shipping_methods").Select("id,shipping_option_id").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("id").Clauses(clause.Locking{Strength: "SHARE"}).Limit(101).Scan(&shipping).Error != nil || len(shipping) > 100 || len(items)+len(shipping) > 1000 {
		return TaxRefreshResult{}, ErrSchema
	}
	for _, row := range shipping {
		if !idPattern.MatchString(row.ID) {
			return TaxRefreshResult{}, ErrSchema
		}
		items = append(items, tax.CalculationItem{Item: tax.Item{ID: row.ID, Shipping: true, ShippingOptionID: row.ShippingOptionID}})
	}
	lines, err := tax.ReadSystem(tx, context, items)
	if err != nil {
		return TaxRefreshResult{}, ErrSchema
	}
	// The source set operation assigns fresh IDs then soft-deletes every old
	// line of this cart, including empty-result and removed-parent cases.
	// Validate SQL storage bounds before the first write; never truncate code.
	for _, line := range lines {
		if utf8.RuneCountInString(line.Code) > 191 || !utf8.ValidString(line.Code) || line.ProviderID != "system" {
			return TaxRefreshResult{}, ErrSchema
		}
	}
	if tx.Exec(`UPDATE cart_line_tax_lines SET deleted_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6) WHERE deleted_at IS NULL AND item_id IN (SELECT id FROM cart_line_items WHERE cart_id=?)`, cart.ID).Error != nil ||
		tx.Exec(`UPDATE cart_shipping_tax_lines SET deleted_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6) WHERE deleted_at IS NULL AND shipping_method_id IN (SELECT id FROM cart_shipping_methods WHERE cart_id=?)`, cart.ID).Error != nil {
		return TaxRefreshResult{}, ErrStorage
	}
	var result TaxRefreshResult
	for _, line := range lines {
		var query, parent, prefix string
		if line.LineItemID != "" {
			query = `INSERT INTO cart_line_tax_lines(id,description,tax_rate_id,code,rate,provider_id,item_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`
			parent, prefix = line.LineItemID, "calitxl_"
			result.ItemLines++
		} else {
			query = `INSERT INTO cart_shipping_tax_lines(id,description,tax_rate_id,code,rate,provider_id,shipping_method_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`
			parent, prefix = line.ShippingLineID, "casmtxl_"
			result.ShippingLines++
		}
		if tx.Exec(query, prefix+uuid.NewString(), line.Name, line.RateID, line.Code, line.Rate, line.ProviderID, parent).Error != nil {
			return TaxRefreshResult{}, ErrStorage
		}
	}
	return result, nil
}
