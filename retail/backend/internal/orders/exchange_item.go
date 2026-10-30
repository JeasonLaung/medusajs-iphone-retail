package orders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/pricing"
	"github.com/eurorefurb/retail/internal/tax"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrExchangeItem = errors.New("exchange item snapshot unavailable")

type ExchangeItem struct {
	ItemID, DetailID, VariantID, UnitPrice string
	Version, TaxLines                      int
	TaxInclusive                           bool
}

// CreateExchangeItemTx creates one outbound same-SKU item in the caller's
// authorized, locked exchange transaction. OriginalItem is a current order
// line, never a browser SKU/price. The original custom workflow explicitly
// supplies that line's unit price, but uses fresh catalog/tax configuration.
// This participant does not commit or make exchange/return/reservation facts.
// Caller MUST provide immutable replay and roll back the WHOLE transaction on
// error, finish native changes, promotion handling, amounts and reservations.
func CreateExchangeItemTx(ctx context.Context, tx *gorm.DB, orderID, originalItem string, version int) (out ExchangeItem, err error) {
	stage := "input"
	defer func() {
		if err != nil {
			err = fmt.Errorf("exchange item %s: %w", stage, err)
		}
	}()
	if !checkoutID.MatchString(orderID) || !checkoutID.MatchString(originalItem) || version < 1 {
		return out, ErrExchangeItem
	}
	if numberTransaction(ctx, tx) != nil {
		return out, ErrExchangeItem
	}
	var order struct {
		Version                                     int
		CurrencyCode                                string
		SalesChannelID, RegionID, ShippingAddressID *string
		Status                                      string
		IsDraftOrder                                bool
		CanceledAt                                  *time.Time
	}
	if tx.Table("orders").Select("version,currency_code,sales_channel_id,region_id,shipping_address_id,status,is_draft_order,canceled_at").Where("id=? AND deleted_at IS NULL", orderID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&order).Error != nil || order.Version != version || order.SalesChannelID == nil || order.RegionID == nil || order.Status == "canceled" || order.Status == "draft" || order.IsDraftOrder || order.CanceledAt != nil {
		return out, ErrExchangeItem
	}
	var channelCount int64
	if tx.Table("commerce_sales_channels").Where("id=? AND is_disabled=FALSE AND deleted_at IS NULL", *order.SalesChannelID).Count(&channelCount).Error != nil || channelCount != 1 {
		return out, ErrExchangeItem
	}
	var originalCount int64
	if tx.Table("order_items").Where("order_id=? AND item_id=? AND version=? AND deleted_at IS NULL", orderID, originalItem, version).Count(&originalCount).Error != nil || originalCount != 1 {
		return out, ErrExchangeItem
	}
	stage = "original line"
	var old struct {
		VariantID, UnitPrice string
		RawUnitPrice         *string
	}
	if tx.Table("order_items i").Select("l.variant_id,CAST(COALESCE(i.unit_price,l.unit_price) AS CHAR) AS unit_price,COALESCE(i.raw_unit_price,l.raw_unit_price) AS raw_unit_price").Joins("JOIN order_line_items l ON l.id=i.item_id AND l.deleted_at IS NULL").Where("i.order_id=? AND i.item_id=? AND i.version=? AND i.deleted_at IS NULL", orderID, originalItem, version).Take(&old).Error != nil {
		return out, ErrExchangeItem
	}
	price, e := amountNumber(old.UnitPrice, old.RawUnitPrice)
	if e != nil {
		return out, e
	}
	effective, e := totals.Resolve(price)
	if e != nil || strings.HasPrefix(effective, "-") {
		return out, ErrExchangeItem
	}
	scalar, raw, e := exchangeNumber(effective)
	if e != nil {
		return out, e
	}
	stage = "catalog"
	variants, e := catalog.ReadCartVariants(tx, []string{old.VariantID}, *order.SalesChannelID)
	if e != nil {
		return out, ErrExchangeItem
	}
	v, ok := variants[old.VariantID]
	if !ok || !v.ManageInventory || v.AllowBackorder || v.IsGiftcard {
		return out, ErrExchangeItem
	}
	region := ""
	if order.RegionID != nil {
		region = *order.RegionID
	}
	// A supplied custom price is valid without a current quote. If present,
	// the current calculated quote controls the inclusive flag, as in source.
	stage = "pricing"
	quotes, e := pricing.Read(tx, []string{v.ID}, pricing.Context{Currency: order.CurrencyCode, Region: region, Channel: *order.SalesChannelID, Quantity: "1"}, time.Now().UTC())
	if e != nil {
		return out, ErrExchangeItem
	}
	inclusive := false
	var compareScalar, compareRaw any
	if q := quotes[v.ID]; q != nil {
		inclusive = q.CalculatedTaxInclusive
		if q.CalculatedKind.Type != nil && *q.CalculatedKind.Type == "sale" && q.OriginalAmount != nil && *q.OriginalAmount != q.Amount {
			a, b, e := exchangeNumber(*q.OriginalAmount)
			if e != nil {
				return out, e
			}
			compareScalar, compareRaw = a, b
		}
	}
	id, detail := "ordli_"+uuid.NewString(), "orditem_"+uuid.NewString()
	stage = "tax context"
	context := tax.CalculationContext{}
	if order.RegionID != nil {
		var r struct{ AutomaticTaxes bool }
		if tx.Table("commerce_regions").Select("automatic_taxes").Where("id=? AND deleted_at IS NULL", *order.RegionID).Take(&r).Error != nil {
			return out, ErrExchangeItem
		}
		context.Automatic = r.AutomaticTaxes
	}
	if order.ShippingAddressID != nil {
		var a struct{ CountryCode, Province string }
		if tx.Table("order_addresses").Select("COALESCE(country_code,'') AS country_code,COALESCE(province,'') AS province").Where("id=? AND deleted_at IS NULL", *order.ShippingAddressID).Take(&a).Error != nil {
			return out, ErrExchangeItem
		}
		context.Country, context.Province = a.CountryCode, a.Province
	}
	stage = "tax selection"
	lines, e := tax.ReadSystem(tx, context, []tax.CalculationItem{{Item: tax.Item{ID: id, ProductID: &v.ProductID, ProductTypeID: v.ProductTypeID}, Giftcard: v.IsGiftcard}})
	if e != nil {
		return out, ErrExchangeItem
	}
	stage = "line insert"
	if tx.Exec(`INSERT INTO order_line_items(id,totals_id,title,subtitle,thumbnail,variant_id,product_id,product_title,product_description,product_subtitle,product_type,product_collection,product_handle,variant_sku,variant_barcode,variant_title,variant_option_values,requires_shipping,is_discountable,is_tax_inclusive,unit_price,raw_unit_price,compare_at_unit_price,raw_compare_at_unit_price,metadata,created_at,updated_at,product_type_id,is_custom_price,is_giftcard)
 VALUES(?,NULL,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'{}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),?,TRUE,FALSE)`, id, v.ProductTitle, v.Title, v.Thumbnail, v.ID, v.ProductID, v.ProductTitle, v.ProductDescription, v.ProductSubtitle, v.ProductType, v.ProductCollection, v.ProductHandle, v.SKU, v.Barcode, v.Title, v.OptionValues, v.RequiresShipping, v.Discountable, inclusive, scalar, raw, compareScalar, compareRaw, v.ProductTypeID).Error != nil {
		return out, ErrExchangeItem
	}
	stage = "detail insert"
	row := map[string]any{"id": detail, "order_id": orderID, "version": version, "item_id": id, "unit_price": scalar, "raw_unit_price": raw, "compare_at_unit_price": compareScalar, "raw_compare_at_unit_price": compareRaw, "created_at": time.Now().UTC(), "updated_at": time.Now().UTC()}
	for _, field := range []string{"quantity", "fulfilled_quantity", "shipped_quantity", "return_requested_quantity", "return_received_quantity", "return_dismissed_quantity", "written_off_quantity", "delivered_quantity"} {
		value := "0"
		if field == "quantity" {
			value = "1"
		}
		row[field] = value
		row["raw_"+field] = `{"value":"` + value + `","precision":100}`
	}
	if tx.Table("order_items").Create(row).Error != nil {
		return out, ErrExchangeItem
	}
	if tx.Table("order_line_items").Where("id=?", id).Update("totals_id", detail).Error != nil {
		return out, ErrExchangeItem
	}
	stage = "tax insert"
	for _, line := range lines {
		rate, raw, e := exchangeNumber(line.Rate)
		if e != nil {
			return out, e
		}
		if tx.Exec(`INSERT INTO order_line_tax_lines(id,item_id,description,tax_rate_id,code,rate,raw_rate,provider_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, "ordtax_"+uuid.NewString(), id, line.Name, line.RateID, line.Code, rate, raw, line.ProviderID).Error != nil {
			return out, ErrExchangeItem
		}
	}
	return ExchangeItem{ItemID: id, DetailID: detail, VariantID: v.ID, UnitPrice: effective, Version: version, TaxLines: len(lines), TaxInclusive: inclusive}, nil
}

// Keep the effective arbitrary precision price in raw; truncate only its SQL
// DECIMAL(38,18) compatibility projection, never ask MySQL to round it.
func exchangeNumber(value string) (string, string, error) {
	v, e := totals.Resolve(totals.Number{Decimal: value})
	if e != nil || strings.HasPrefix(v, "-") {
		return "", "", ErrExchangeItem
	}
	parts := strings.Split(v, ".")
	if len(parts[0]) > 20 {
		return "", "", ErrExchangeItem
	}
	scalar := parts[0]
	if len(parts) == 2 {
		f := parts[1]
		if len(f) > 18 {
			f = f[:18]
		}
		scalar += "." + f
	}
	raw, e := json.Marshal(totals.RawValue{Value: v, Precision: 100})
	if e != nil {
		return "", "", ErrExchangeItem
	}
	return scalar, string(raw), nil
}
