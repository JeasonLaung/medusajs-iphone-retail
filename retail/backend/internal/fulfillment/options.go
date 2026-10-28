package fulfillment

import (
	"encoding/json"
	"errors"
	"unicode/utf8"

	"gorm.io/gorm"
)

var ErrOptions = errors.New("shipping option selection unavailable or invalid")

// OptionSelection is supplied by the trusted cart coordinator after checking
// current possession/version/shop. It is never a browser-defined SQL filter.
type OptionSelection struct {
	Channel        string
	Address        ShippingAddress
	Context        json.RawMessage
	Return         bool
	EnabledInStore *bool
}

type ShippingAddress struct{ Country, Province, City, Postal string }

// Candidate is an internal directory result, not an offer: callers still need
// item profile coverage, provider/configuration checks, inventory and pricing.
// Keep provider/data private until a provider-specific projection is reviewed.
type Candidate struct {
	ID, Name, PriceType, ServiceZoneID, FulfillmentSetID string
	ShippingProfileID, ProviderID                        *string
	ProviderEnabled                                      bool
	Data                                                 *string
	LocationIDs                                          []string `gorm:"-"`
}

// ReadCandidates reads one caller-owned consistent transaction. Missing channel
// and all soft-deleted relationship edges produce no candidates. Invalid rules,
// oversized directories or SQL errors fail the entire selection, never truncate.
func ReadCandidates(tx *gorm.DB, input OptionSelection) ([]Candidate, error) {
	if tx == nil || tx.Statement == nil {
		return nil, ErrOptions
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, ErrOptions
	}
	if input.Channel == "" || len(input.Channel) > 191 || !utf8.ValidString(input.Channel) {
		return nil, ErrOptions
	}
	for _, v := range []string{input.Address.Country, input.Address.Province, input.Address.City, input.Address.Postal} {
		if len(v) > 4096 || !utf8.ValidString(v) {
			return nil, ErrOptions
		}
	}
	if err := Ready(tx.Statement.Context, tx); err != nil {
		return nil, err
	}
	if err := GeoReady(tx.Statement.Context, tx); err != nil {
		return nil, err
	}
	context := map[string]json.RawMessage{}
	if len(input.Context) > 1<<20 {
		return nil, ErrOptions
	}
	if len(input.Context) > 0 {
		if json.Unmarshal(input.Context, &context) != nil || context == nil {
			return nil, ErrOptions
		}
	}
	context["is_return"] = json.RawMessage(`"false"`)
	if input.Return {
		context["is_return"] = json.RawMessage(`"true"`)
	}
	context["enabled_in_store"] = json.RawMessage(`"true"`)
	if input.EnabledInStore != nil && !*input.EnabledInStore {
		context["enabled_in_store"] = json.RawMessage(`"false"`)
	}
	normalized, err := json.Marshal(context)
	if err != nil {
		return nil, ErrOptions
	}
	var links []struct{ FulfillmentSetID, StockLocationID string }
	q := tx.Table("commerce_channel_locations cl").Select("ls.fulfillment_set_id,cl.stock_location_id").
		Joins("JOIN commerce_sales_channels c ON c.id=cl.sales_channel_id AND c.deleted_at IS NULL AND c.is_disabled=FALSE").
		Joins("JOIN inventory_locations l ON l.id=cl.stock_location_id AND l.deleted_at IS NULL").
		Joins("JOIN fulfillment_location_sets ls ON ls.stock_location_id=l.id AND ls.deleted_at IS NULL").
		Joins("JOIN fulfillment_sets fs ON fs.id=ls.fulfillment_set_id AND fs.deleted_at IS NULL").
		Where("cl.sales_channel_id=? AND cl.deleted_at IS NULL", input.Channel).Order("ls.fulfillment_set_id,cl.stock_location_id").Limit(1001)
	if q.Scan(&links).Error != nil || len(links) > 1000 {
		return nil, ErrOptions
	}
	out := []Candidate{}
	if len(links) == 0 {
		return out, nil
	}
	sets := []string{}
	locations := map[string][]string{}
	for _, link := range links {
		if _, ok := locations[link.FulfillmentSetID]; !ok {
			sets = append(sets, link.FulfillmentSetID)
		}
		locations[link.FulfillmentSetID] = append(locations[link.FulfillmentSetID], link.StockLocationID)
	}
	var rows []Candidate
	query := tx.Table("fulfillment_shipping_options o").Select("o.id,o.name,o.price_type,o.service_zone_id,z.fulfillment_set_id,o.shipping_profile_id,o.provider_id,o.data,COALESCE(p.is_enabled,FALSE) AS provider_enabled").
		Joins("JOIN fulfillment_service_zones z ON z.id=o.service_zone_id AND z.deleted_at IS NULL").
		Joins("JOIN fulfillment_option_types t ON t.id=o.shipping_option_type_id AND t.deleted_at IS NULL").
		Joins("LEFT JOIN fulfillment_providers p ON p.id=o.provider_id AND p.deleted_at IS NULL").
		Where("z.fulfillment_set_id IN ? AND o.deleted_at IS NULL", sets)
	// The source only constrains properties present on the address, with OR
	// branches for each present granularity. Empty address adds no geo predicate.
	a := input.Address
	branches := []string{}
	args := []any{}
	for _, kind := range []string{"zip", "city", "province", "country"} {
		present := map[string]string{"zip": a.Postal, "city": a.City, "province": a.Province, "country": a.Country}[kind]
		if present == "" {
			continue
		}
		part := "g.type=?"
		values := []any{kind}
		fields := []struct{ column, value string }{{"country_code", a.Country}}
		if kind != "country" {
			fields = append(fields, struct{ column, value string }{"province_code", a.Province})
		}
		if kind == "zip" || kind == "city" {
			fields = append(fields, struct{ column, value string }{"city", a.City})
		}
		for _, field := range fields {
			if field.value != "" {
				part += " AND g." + field.column + "=?"
				values = append(values, field.value)
			}
		}
		if kind == "zip" {
			// Original PostgreSQL JSON column = JSON-string parameter. The
			// stored object/array is not a regex, range or substring expression.
			part += " AND JSON_TYPE(g.postal_expression)='STRING' AND BINARY JSON_UNQUOTE(g.postal_expression)=BINARY ?"
			values = append(values, a.Postal)
		}
		branches = append(branches, "("+part+")")
		args = append(args, values...)
	}
	if len(branches) > 0 {
		predicate := "EXISTS (SELECT 1 FROM fulfillment_geo_zones g WHERE g.service_zone_id=z.id AND g.deleted_at IS NULL AND ("
		for i, branch := range branches {
			if i > 0 {
				predicate += " OR "
			}
			predicate += branch
		}
		query = query.Where(predicate+"))", args...)
	}
	if query.Order("o.id").Limit(1001).Scan(&rows).Error != nil || len(rows) > 1000 {
		return nil, ErrOptions
	}
	if len(rows) == 0 {
		return out, nil
	}
	ids := []string{}
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	var rules []struct {
		ShippingOptionID, Attribute, Operator string
		Value                                 *string
	}
	if tx.Table("fulfillment_option_rules").Select("shipping_option_id,attribute,operator,value").Where("shipping_option_id IN ? AND deleted_at IS NULL", ids).Order("shipping_option_id,id").Limit(10001).Scan(&rules).Error != nil || len(rules) > 10000 {
		return nil, ErrOptions
	}
	byOption := map[string][]Rule{}
	for _, rule := range rules {
		if rule.Value == nil {
			return nil, ErrRules
		}
		byOption[rule.ShippingOptionID] = append(byOption[rule.ShippingOptionID], Rule{Attribute: rule.Attribute, Operator: rule.Operator, Value: json.RawMessage(*rule.Value)})
	}
	for _, row := range rows {
		valid, err := MatchRules(normalized, byOption[row.ID], false)
		if err != nil {
			return nil, err
		}
		if valid {
			row.LocationIDs = append([]string(nil), locations[row.FulfillmentSetID]...)
			out = append(out, row)
		}
	}
	return out, nil
}
