// Package tax selects configured tax rates and projects the built-in system
// provider's lines. It does not read configuration, authorize a cart, persist
// tax lines, or calculate payable totals. Callers supply a trusted snapshot.
package tax

import (
	"errors"
	"sort"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/totals"
)

var ErrInput = errors.New("tax configuration unavailable")

type Item struct {
	ID               string  `json:"id"`
	ProductID        *string `json:"product_id,omitempty"`
	ProductTypeID    *string `json:"product_type_id,omitempty"`
	ShippingOptionID *string `json:"shipping_option_id,omitempty"`
	// Shipping distinguishes a shipping line with a null option from a product.
	Shipping bool `json:"-"`
}

type Region struct {
	ID           string  `json:"id"`
	ProvinceCode *string `json:"province_code"`
	ParentID     *string `json:"parent_id"`
}

type Rule struct {
	Reference   string `json:"reference"`
	ReferenceID string `json:"reference_id"`
}

type Rate struct {
	ID           string  `json:"id"`
	RegionID     string  `json:"tax_region_id" gorm:"column:tax_region_id"`
	Region       Region  `json:"tax_region" gorm:"-"`
	Rate         *string `json:"rate"`
	Name         string  `json:"name"`
	Code         string  `json:"code"`
	IsDefault    bool    `json:"is_default"`
	IsCombinable bool    `json:"is_combinable"`
	Rules        []Rule  `json:"rules" gorm:"-"`
}

func bounded(s string, limit int) bool { return len(s) <= limit && utf8.ValidString(s) }
func id(s string) bool                 { return s != "" && bounded(s, 191) }
func optionalID(s *string) bool        { return s == nil || id(*s) }

func validItem(item Item) bool {
	return id(item.ID) && optionalID(item.ProductID) && optionalID(item.ProductTypeID) && optionalID(item.ShippingOptionID) && (item.Shipping || item.ShippingOptionID == nil)
}

func validRates(rates []Rate) bool {
	if len(rates) > 10000 {
		return false
	}
	seen := make(map[string]bool, len(rates))
	rules := 0
	for _, rate := range rates {
		if !id(rate.ID) || seen[rate.ID] || !id(rate.RegionID) || rate.Region.ID != rate.RegionID || !optionalID(rate.Region.ParentID) || rate.Region.ProvinceCode != nil && !bounded(*rate.Region.ProvinceCode, 191) || !bounded(rate.Name, 16384) || !bounded(rate.Code, 16384) {
			return false
		}
		seen[rate.ID] = true
		if rate.Rate != nil {
			if _, err := totals.Resolve(totals.Number{Decimal: *rate.Rate}); err != nil {
				return false
			}
		}
		rules += len(rate.Rules)
		if rules > 20000 {
			return false
		}
		for _, rule := range rate.Rules {
			// Unknown references remain configuration, but never match an item.
			if !id(rule.ReferenceID) || !bounded(rule.Reference, 191) {
				return false
			}
		}
	}
	return true
}

func matches(rule Rule, item Item) bool {
	if item.Shipping {
		return rule.Reference == "shipping_option" && item.ShippingOptionID != nil && rule.ReferenceID == *item.ShippingOptionID
	}
	return rule.Reference == "product" && item.ProductID != nil && rule.ReferenceID == *item.ProductID || rule.Reference == "product_type" && item.ProductTypeID != nil && rule.ReferenceID == *item.ProductTypeID
}

func priority(rate Rate, item Item) (int, bool) {
	product, kind, shipping := false, false, false
	for _, rule := range rate.Rules {
		if !matches(rule, item) {
			continue
		}
		kind = true
		product = product || rule.Reference == "product"
		shipping = shipping || item.Shipping
	}
	province := rate.Region.ProvinceCode != nil
	switch {
	case (product || shipping) && province:
		return 1, true
	case kind && province:
		return 2, true
	case rate.IsDefault && province:
		return 3, true
	case product || shipping:
		return 4, true
	case kind:
		return 5, true
	case rate.IsDefault:
		return 6, true
	default:
		return 7, false
	}
}

func copyString(s *string) *string {
	if s == nil {
		return nil
	}
	value := *s
	return &value
}
func copyRate(rate Rate) Rate {
	rate.Rate = copyString(rate.Rate)
	rate.Region.ProvinceCode = copyString(rate.Region.ProvinceCode)
	rate.Region.ParentID = copyString(rate.Region.ParentID)
	if rate.Rules != nil {
		rate.Rules = append([]Rule{}, rate.Rules...)
	}
	return rate
}

// Select first applies the original region/default/reference candidate query,
// then stable priority. Exactly one parent rate may be combined; it does not
// recurse through ancestors or add all matching rates. Input order is evidence:
// callers must preserve it and report same-priority configuration ambiguity.
func Select(item Item, rates []Rate, regionIDs []string) ([]Rate, error) {
	if !validItem(item) || !validRates(rates) || len(regionIDs) > 1000 {
		return nil, ErrInput
	}
	regions := make(map[string]bool, len(regionIDs))
	for _, region := range regionIDs {
		if !id(region) || regions[region] {
			return nil, ErrInput
		}
		regions[region] = true
	}
	type candidate struct{ index, score int }
	candidates := []candidate{}
	for i, rate := range rates {
		if !regions[rate.RegionID] {
			continue
		}
		score, match := priority(rate, item)
		if match {
			candidates = append(candidates, candidate{i, score})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score < candidates[j].score })
	result := []Rate{}
	if len(candidates) == 0 {
		return result, nil
	}
	first := rates[candidates[0].index]
	result = append(result, copyRate(first))
	if first.IsCombinable && first.Region.ParentID != nil && *first.Region.ParentID != "" {
		for _, row := range candidates {
			if rates[row.index].Region.ID == *first.Region.ParentID {
				result = append(result, copyRate(rates[row.index]))
				break
			}
		}
	}
	return result, nil
}

type Group struct {
	Item  Item
	Rates []Rate
}

type Line struct {
	RateID         string `json:"rate_id"`
	Rate           string `json:"rate"`
	Name           string `json:"name"`
	Code           string `json:"code"`
	LineItemID     string `json:"line_item_id,omitempty"`
	ShippingLineID string `json:"shipping_line_id,omitempty"`
	ProviderID     string `json:"provider_id"`
}

// SystemLines is the built-in system provider's pure projection. Invoke only
// after resolving that provider, filtering gift cards and selecting rates.
// It cannot stand in for another configured provider. Null rate means zero;
// exact configured decimals pass through Resolve without float conversion.
func SystemLines(groups []Group) ([]Line, error) {
	if len(groups) > 10000 {
		return nil, ErrInput
	}
	result := []Line{}
	seen := map[string]bool{}
	for _, shipping := range []bool{false, true} {
		for _, group := range groups {
			if group.Item.Shipping != shipping {
				continue
			}
			if !validItem(group.Item) || !validRates(group.Rates) || seen[group.Item.ID] || len(result)+len(group.Rates) > 20000 {
				return nil, ErrInput
			}
			seen[group.Item.ID] = true
			for _, rate := range group.Rates {
				value := "0"
				if rate.Rate != nil {
					var err error
					value, err = totals.Resolve(totals.Number{Decimal: *rate.Rate})
					if err != nil {
						return nil, ErrInput
					}
				}
				line := Line{RateID: rate.ID, Rate: value, Name: rate.Name, Code: rate.Code, ProviderID: "system"}
				if shipping {
					line.ShippingLineID = group.Item.ID
				} else {
					line.LineItemID = group.Item.ID
				}
				result = append(result, line)
			}
		}
	}
	return result, nil
}
