package cart

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
)

// PromotionTargets is private rule input, including saved metadata/provider
// fields selected by the original internal graph. Never return it as an HTTP
// cart DTO. It is not the complete order/address/customer promotion context.
type PromotionTargets struct {
	Items           []map[string]any `json:"items"`
	ShippingMethods []map[string]any `json:"shipping_methods"`
}

// ReadPromotionTargets rechecks current possession/version and assembles target
// facts in the caller's transaction. It preserves existing adjustments and their
// discounted totals; subtotal/original_total remain undiscounted allocator bases.
// It does not refresh prices/taxes, commit, mutate, or grant checkout authority.
func ReadPromotionTargets(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string) (PromotionTargets, error) {
	failed := PromotionTargets{}
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return failed, err
	}
	amounts, err := assembleAmounts(tx, cart)
	if err != nil {
		return failed, err
	}
	items, err := promotionRows(tx, "cart_line_items", "cart_id", []string{cart.ID}, 1000, promotionFields{
		text:   "id cart_id title subtitle thumbnail variant_id product_id product_title product_description product_subtitle product_type product_collection product_handle variant_sku variant_barcode variant_title product_type_id",
		number: "quantity compare_at_unit_price unit_price", boolean: "requires_shipping is_discountable is_tax_inclusive is_custom_price is_giftcard",
		json: "variant_option_values raw_compare_at_unit_price raw_unit_price metadata", date: "created_at updated_at deleted_at",
	})
	if err != nil {
		return failed, err
	}
	shipping, err := promotionRows(tx, "cart_shipping_methods", "cart_id", []string{cart.ID}, 100, promotionFields{
		text: "id cart_id name shipping_option_id", number: "amount", boolean: "is_tax_inclusive", json: "description raw_amount data metadata", date: "created_at updated_at deleted_at",
	})
	if err != nil {
		return failed, err
	}
	if len(items) != len(amounts.Lines) || len(shipping) != len(amounts.ShippingMethods) {
		return failed, ErrSchema
	}
	productIDs, variantIDs, optionIDs := []string{}, []string{}, []string{}
	for i, row := range items {
		if row["id"] != amounts.Lines[i].ID {
			return failed, ErrSchema
		}
		row["unit_price"] = json.Number(amounts.Lines[i].UnitPrice)
		if row["compare_at_unit_price"] != nil || row["raw_compare_at_unit_price"] != nil {
			if err := promotionEffective(row, "compare_at_unit_price"); err != nil {
				return failed, err
			}
		}
		promotionTotals(row, amounts.Lines[i].Totals)
		if id, ok := row["product_id"].(string); ok {
			productIDs = append(productIDs, id)
		}
		if id, ok := row["variant_id"].(string); ok {
			variantIDs = append(variantIDs, id)
		}
	}
	products, err := catalog.ReadPromotionProducts(tx, productIDs)
	if err != nil {
		return failed, err
	}
	variants, err := catalog.ReadPromotionVariants(tx, variantIDs)
	if err != nil {
		return failed, err
	}
	for _, row := range items {
		row["product"], row["variant"] = nil, nil
		if id, ok := row["product_id"].(string); ok {
			if p, found := products[id]; found {
				row["product"] = p
			}
		}
		if id, ok := row["variant_id"].(string); ok {
			if v, found := variants[id]; found {
				row["variant"] = v
			}
		}
	}
	for i, row := range shipping {
		if row["id"] != amounts.ShippingMethods[i].ID {
			return failed, ErrSchema
		}
		row["amount"] = json.Number(amounts.ShippingMethods[i].Amount)
		promotionTotals(row, amounts.ShippingMethods[i].Totals)
		if id, ok := row["shipping_option_id"].(string); ok {
			optionIDs = append(optionIDs, id)
		}
	}
	// The graph selects option scalar foreign keys, not the linked type/profile
	// entities. Keep those IDs even if a referenced entity is missing/deleted.
	options, err := promotionRows(tx, "fulfillment_shipping_options", "id", optionIDs, 100, promotionFields{text: "id shipping_option_type_id shipping_profile_id"})
	if err != nil {
		return failed, err
	}
	byOption := map[string]map[string]any{}
	for _, row := range options {
		id, ok := row["id"].(string)
		if !ok {
			return failed, ErrSchema
		}
		delete(row, "id")
		byOption[id] = row
	}
	for _, row := range shipping {
		row["shipping_option"] = nil
		if id, ok := row["shipping_option_id"].(string); ok {
			if option, found := byOption[id]; found {
				row["shipping_option"] = option
			}
		}
	}
	for _, spec := range []struct {
		rows                    []map[string]any
		parent, adjustment, tax string
		inclusive               bool
	}{
		{items, "item_id", "cart_line_adjustments", "cart_line_tax_lines", true},
		{shipping, "shipping_method_id", "cart_shipping_adjustments", "cart_shipping_tax_lines", false},
	} {
		ids := []string{}
		byID := map[string]map[string]any{}
		for _, row := range spec.rows {
			id, ok := row["id"].(string)
			if !ok || !idPattern.MatchString(id) {
				return failed, ErrSchema
			}
			ids = append(ids, id)
			byID[id] = row
			row["adjustments"], row["tax_lines"] = []map[string]any{}, []map[string]any{}
		}
		adjustmentFields := promotionFields{text: "id description promotion_id code provider_id " + spec.parent, number: "amount", json: "raw_amount metadata", date: "created_at updated_at deleted_at"}
		if spec.inclusive {
			adjustmentFields.boolean = "is_tax_inclusive"
		}
		adjustments, err := promotionRows(tx, spec.adjustment, spec.parent, ids, len(ids)*100, adjustmentFields)
		if err != nil {
			return failed, err
		}
		taxes, err := promotionRows(tx, spec.tax, spec.parent, ids, len(ids)*100, promotionFields{text: "id description tax_rate_id code provider_id " + spec.parent, number: "rate", json: "metadata data", date: "created_at updated_at deleted_at"})
		if err != nil {
			return failed, err
		}
		for _, group := range []struct {
			key  string
			rows []map[string]any
		}{{"adjustments", adjustments}, {"tax_lines", taxes}} {
			for _, relation := range group.rows {
				parent, ok := relation[spec.parent].(string)
				if !ok || byID[parent] == nil {
					return failed, ErrSchema
				}
				if group.key == "adjustments" {
					if err := promotionEffective(relation, "amount"); err != nil {
						return failed, err
					}
				}
				list := byID[parent][group.key].([]map[string]any)
				if len(list) >= 100 {
					return failed, ErrSchema
				}
				byID[parent][group.key] = append(list, relation)
			}
		}
	}
	out := PromotionTargets{Items: items, ShippingMethods: shipping}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > 1<<20 {
		return failed, ErrSchema
	}
	return out, nil
}

// Only fixed internal table/column specifications call this helper. Casting
// numbers to SQL text avoids map scanning through float64. Explicit field lists
// prevent a future schema column from silently becoming a promotion fact.
type promotionFields struct{ text, number, boolean, json, date string }

func promotionRows(tx *gorm.DB, table, parent string, ids []string, limit int, fields promotionFields) ([]map[string]any, error) {
	out := []map[string]any{}
	if len(ids) == 0 {
		return out, nil
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if !seen[id] {
			unique = append(unique, id)
			seen[id] = true
		}
	}
	projection := []string{}
	kinds := map[string]int{}
	for kind, names := range []string{fields.text, fields.number, fields.boolean, fields.json, fields.date} {
		for _, name := range strings.Fields(names) {
			if _, exists := kinds[name]; exists {
				return nil, ErrSchema
			}
			kinds[name] = kind
			selectField := "CAST(`" + name + "` AS CHAR)"
			if kind == 4 {
				selectField = "DATE_FORMAT(`" + name + "`,'%Y-%m-%dT%H:%i:%s.%fZ')"
			}
			projection = append(projection, selectField+" AS `"+name+"`")
		}
	}
	if tx.Table(table).Select(strings.Join(projection, ",")).Where(parent+" IN ? AND deleted_at IS NULL", unique).Order("id").Limit(limit+1).Find(&out).Error != nil || len(out) > limit {
		return nil, ErrSchema
	}
	for _, row := range out {
		for name, kind := range kinds {
			value, exists := row[name]
			if !exists {
				return nil, ErrSchema
			}
			if value == nil {
				continue
			}
			var s string
			switch v := value.(type) {
			case string:
				s = v
			case []byte:
				s = string(v)
			default:
				return nil, ErrSchema
			}
			if len(s) > 1<<20 {
				return nil, ErrSchema
			}
			switch kind {
			case 0:
				row[name] = s
			case 1:
				n, err := totals.Resolve(totals.Number{Decimal: s})
				if err != nil {
					return nil, ErrSchema
				}
				row[name] = json.Number(n)
			case 2:
				if s != "0" && s != "1" {
					return nil, ErrSchema
				}
				row[name] = s == "1"
			case 3:
				if strings.HasPrefix(name, "raw_") {
					if _, err := totals.StoredNumber("0", &s); err != nil {
						return nil, ErrSchema
					}
				}
				decoder := json.NewDecoder(bytes.NewBufferString(s))
				decoder.UseNumber()
				var decoded any
				if decoder.Decode(&decoded) != nil || decoder.Decode(new(any)) != io.EOF {
					return nil, ErrSchema
				}
				row[name] = decoded
			case 4:
				// JS Date (the original graph) has millisecond resolution.
				if len(s) != 27 {
					return nil, ErrSchema
				}
				row[name] = s[:23] + "Z"
			}
		}
	}
	return out, nil
}

func promotionEffective(row map[string]any, field string) error {
	scalar, ok := row[field].(json.Number)
	if !ok && row[field] != nil {
		return ErrSchema
	}
	if !ok {
		scalar = json.Number("0")
	}
	var raw *string
	if value := row["raw_"+field]; value != nil {
		encoded, err := json.Marshal(value)
		if err != nil {
			return ErrSchema
		}
		s := string(encoded)
		raw = &s
	}
	n, err := nonNegativeAmount(string(scalar), raw)
	if err != nil {
		return err
	}
	value, err := totals.Resolve(n)
	if err != nil {
		return ErrSchema
	}
	row[field] = json.Number(value)
	return nil
}

func promotionTotals(row map[string]any, values map[string]string) {
	for key, value := range values {
		row[key] = json.Number(value)
	}
}
