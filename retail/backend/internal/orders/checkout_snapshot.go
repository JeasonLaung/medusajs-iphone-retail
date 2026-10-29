package orders

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrCheckoutSnapshot = errors.New("order checkout snapshot unavailable")
var checkoutID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$`)

// CartSnapshotInput is INTERNAL, assembled only after the owning cart workflow
// locks and verifies possession, version, frozen payment binding and paid proof.
// Neither this input nor a raw cart ID is an HTTP authorization mechanism.
type CartSnapshotInput struct {
	CartID       string
	CurrencyCode string
	Totals       map[string]string
}

type CheckoutSnapshot struct {
	OrderID     string
	LineIDs     map[string]string // cart line -> NEW order line, for reservations
	ShippingIDs map[string]string
}

// WriteCartSnapshot participates in an existing order transaction. The owner
// MUST roll back the whole transaction on any error (including later stock,
// promotion, ledger, receipt or notification-outbox failure). Before invoking,
// check the checkout idempotency receipt; this participant does not commit or
// complete a cart, establish customer ownership, call providers or send email.
func WriteCartSnapshot(ctx context.Context, tx *gorm.DB, in CartSnapshotInput) (CheckoutSnapshot, error) {
	fail := func() (CheckoutSnapshot, error) { return CheckoutSnapshot{}, ErrCheckoutSnapshot }
	if !checkoutID.MatchString(in.CartID) || len(in.CurrencyCode) != 3 || len(in.Totals) == 0 || in.Totals["total"] == "" {
		return fail()
	}
	if numberTransaction(ctx, tx) != nil {
		return fail()
	}
	var saved struct {
		CurrencyCode                        string
		ShippingAddressID, BillingAddressID *string
	}
	r := tx.WithContext(ctx).Table("carts").Select("currency_code,shipping_address_id,billing_address_id").Where("id=? AND completed_at IS NULL AND deleted_at IS NULL AND customer_id IS NULL", in.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&saved)
	if r.Error != nil || saved.CurrencyCode != in.CurrencyCode || saved.ShippingAddressID == nil || saved.BillingAddressID == nil {
		return fail()
	}
	// Preserve both value copies even when the source address happens to be the
	// same row. No legacy customer_id or arbitrary cart metadata is transferred.
	addresses := []string{}
	for _, source := range []*string{saved.ShippingAddressID, saved.BillingAddressID} {
		id := "ordaddr_" + uuid.NewString()
		r = tx.Exec(`INSERT INTO order_addresses(id,company,first_name,last_name,address_1,address_2,city,country_code,province,postal_code,phone,created_at,updated_at)
 SELECT ?,company,first_name,last_name,address_1,address_2,city,country_code,province,postal_code,phone,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)
 FROM cart_addresses WHERE id=? AND deleted_at IS NULL`, id, *source)
		if r.Error != nil || r.RowsAffected != 1 {
			return fail()
		}
		addresses = append(addresses, id)
	}
	numbers, err := AllocateOrderNumbers(ctx, tx)
	if err != nil {
		return CheckoutSnapshot{}, err
	}
	out := CheckoutSnapshot{OrderID: "order_" + uuid.NewString(), LineIDs: map[string]string{}, ShippingIDs: map[string]string{}}
	r = tx.Exec(`INSERT INTO orders(id,region_id,display_id,version,sales_channel_id,status,is_draft_order,email,currency_code,shipping_address_id,billing_address_id,no_notification,created_at,updated_at,custom_display_id,locale)
 SELECT ?,region_id,?,1,sales_channel_id,'pending',FALSE,email,currency_code,?,?,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),?,locale FROM carts WHERE id=? AND deleted_at IS NULL AND completed_at IS NULL`, out.OrderID, numbers.DisplayID, addresses[0], addresses[1], numbers.CustomDisplayID, in.CartID)
	if r.Error != nil || r.RowsAffected != 1 {
		return fail()
	}
	var lines []string
	if tx.Table("cart_line_items").Select("id").Where("cart_id=? AND deleted_at IS NULL", in.CartID).Order("id").Limit(1001).Scan(&lines).Error != nil || len(lines) == 0 || len(lines) > 1000 {
		return fail()
	}
	const lineColumns = "title,subtitle,thumbnail,variant_id,product_id,product_title,product_description,product_subtitle,product_type,product_collection,product_handle,variant_sku,variant_barcode,variant_title,variant_option_values,requires_shipping,is_discountable,is_tax_inclusive,compare_at_unit_price,raw_compare_at_unit_price,unit_price,raw_unit_price,metadata,is_custom_price,product_type_id,is_giftcard"
	const zero = `'{"value":"0","precision":20}'`
	for _, source := range lines {
		id := "ordli_" + uuid.NewString()
		item := "orditem_" + uuid.NewString()
		r = tx.Exec("INSERT INTO order_line_items(id,"+lineColumns+",created_at,updated_at) SELECT ?,"+lineColumns+",UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) FROM cart_line_items WHERE id=? AND cart_id=? AND deleted_at IS NULL", id, source, in.CartID)
		if r.Error != nil || r.RowsAffected != 1 {
			return fail()
		}
		columns := "id,order_id,version,item_id,quantity,raw_quantity,unit_price,raw_unit_price,compare_at_unit_price,raw_compare_at_unit_price,created_at,updated_at"
		values := "?,?,1,?,quantity,JSON_OBJECT('value',CAST(quantity AS CHAR),'precision',20),unit_price,raw_unit_price,compare_at_unit_price,raw_compare_at_unit_price,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)"
		for _, field := range []string{"fulfilled_quantity", "shipped_quantity", "return_requested_quantity", "return_received_quantity", "return_dismissed_quantity", "written_off_quantity", "delivered_quantity"} {
			columns += "," + field + ",raw_" + field
			values += ",0," + zero
		}
		r = tx.Exec("INSERT INTO order_items("+columns+") SELECT "+values+" FROM cart_line_items WHERE id=? AND cart_id=? AND deleted_at IS NULL AND quantity BETWEEN 1 AND 1000000000", item, out.OrderID, id, source, in.CartID)
		if r.Error != nil || r.RowsAffected != 1 {
			return fail()
		}
		if tx.Table("order_line_items").Where("id=?", id).Update("totals_id", item).Error != nil {
			return fail()
		}
		if err = copyCheckoutRelations(tx, "cart_line_adjustments", "order_line_adjustments", "item_id", source, id, "description,promotion_id,code,amount,raw_amount,provider_id,is_tax_inclusive", true, false); err != nil {
			return fail()
		}
		if err = copyCheckoutRelations(tx, "cart_line_tax_lines", "order_line_tax_lines", "item_id", source, id, "description,tax_rate_id,code,rate,provider_id,metadata,data", false, true); err != nil {
			return fail()
		}
		out.LineIDs[source] = id
	}
	var methods []string
	if tx.Table("cart_shipping_methods").Select("id").Where("cart_id=? AND deleted_at IS NULL", in.CartID).Order("id").Limit(101).Scan(&methods).Error != nil || len(methods) > 100 {
		return fail()
	}
	const shippingColumns = "name,description,amount,raw_amount,is_tax_inclusive,shipping_option_id,data,metadata"
	for _, source := range methods {
		id := "ordsm_" + uuid.NewString()
		r = tx.Exec("INSERT INTO order_shipping_methods(id,"+shippingColumns+",created_at,updated_at,is_custom_amount) SELECT ?,"+shippingColumns+",UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),FALSE FROM cart_shipping_methods WHERE id=? AND cart_id=? AND deleted_at IS NULL", id, source, in.CartID)
		if r.Error != nil || r.RowsAffected != 1 {
			return fail()
		}
		if tx.Table("order_shipping_versions").Create(map[string]any{"id": "ordsv_" + uuid.NewString(), "order_id": out.OrderID, "version": 1, "shipping_method_id": id, "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return fail()
		}
		if err = copyCheckoutRelations(tx, "cart_shipping_adjustments", "order_shipping_adjustments", "shipping_method_id", source, id, "description,promotion_id,code,amount,raw_amount,provider_id", true, false); err != nil {
			return fail()
		}
		if err = copyCheckoutRelations(tx, "cart_shipping_tax_lines", "order_shipping_tax_lines", "shipping_method_id", source, id, "description,tax_rate_id,code,rate,provider_id,metadata,data", false, true); err != nil {
			return fail()
		}
		out.ShippingIDs[source] = id
	}
	var promotions []string
	if tx.Table("cart_promotions").Select("promotion_id").Where("cart_id=? AND deleted_at IS NULL", in.CartID).Order("promotion_id").Limit(101).Scan(&promotions).Error != nil || len(promotions) > 100 {
		return fail()
	}
	for _, id := range promotions {
		if tx.Table("order_promotions").Create(map[string]any{"id": uuid.NewString(), "order_id": out.OrderID, "promotion_id": id, "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return fail()
		}
	}
	if tx.Table("order_carts").Create(map[string]any{"id": uuid.NewString(), "order_id": out.OrderID, "cart_id": in.CartID, "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
		return fail()
	}
	// Verify every financial total via the SAME order reader used by Admin.
	// Raw monetary strings, rather than rounded scalar copies, remain authority.
	amounts, err := ReadAmountSnapshot(tx, out.OrderID)
	if err != nil || amounts.CurrencyCode != in.CurrencyCode {
		return fail()
	}
	for name, want := range in.Totals {
		if got, ok := amounts.Totals[name]; !ok || got != want {
			return fail()
		}
	}
	return out, nil
}

// All identifiers are compile-time constants supplied by WriteCartSnapshot.
// Per-parent bounds mirror the cart/order financial readers. Each row receives
// a new identity so future cart edits can never change a committed order copy.
func copyCheckoutRelations(tx *gorm.DB, from, to, parent, source, target, columns string, version, tax bool) error {
	var ids []string
	if tx.Table(from).Select("id").Where(parent+"=? AND deleted_at IS NULL", source).Order("id").Limit(101).Scan(&ids).Error != nil || len(ids) > 100 {
		return ErrCheckoutSnapshot
	}
	for _, id := range ids {
		destination := "id," + parent + "," + columns + ",created_at,updated_at"
		values := "?,?," + columns + ",UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)"
		if version {
			destination += ",version"
			values += ",1"
		}
		if tax {
			destination += ",raw_rate"
			values += ",JSON_OBJECT('value',CAST(rate AS CHAR),'precision',20)"
		}
		r := tx.Exec("INSERT INTO "+to+"("+destination+") SELECT "+values+" FROM "+from+" WHERE id=? AND "+parent+"=? AND deleted_at IS NULL", strings.TrimPrefix(to, "order_")+"_"+uuid.NewString(), target, id, source)
		if r.Error != nil || r.RowsAffected != 1 {
			return ErrCheckoutSnapshot
		}
	}
	return nil
}
