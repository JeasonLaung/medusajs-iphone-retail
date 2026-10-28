package fulfillment

import (
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/pricing"
	"gorm.io/gorm"
)

// ManualFlatQuote is an internal quote, not a reservation or checkout grant.
// Locations contain only active channel/set/provider associations. Cart-wide
// inventory and complete profile coverage must still be checked before saving.
type ManualFlatQuote struct {
	Option Candidate
	Price  pricing.Quote
}

// ReadManualFlatQuotes implements the configured manual provider's fixed-price
// path. manualProviderIDs come from trusted server registration, never inferred
// from an ID prefix or supplied by the browser. profileIDs must be resolved
// from the current cart's shipping products by the owning cart coordinator.
// Unknown/calculated providers are not executed or represented as zero prices.
func ReadManualFlatQuotes(tx *gorm.DB, selection OptionSelection, profileIDs, manualProviderIDs []string, context pricing.Context, at time.Time) ([]ManualFlatQuote, error) {
	if tx == nil || tx.Statement == nil || context.Channel != selection.Channel || context.Quantity != "" || at.IsZero() {
		return nil, ErrOptions
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, ErrOptions
	}
	validateIDs := func(ids []string, limit int) (map[string]bool, error) {
		if len(ids) > limit {
			return nil, ErrOptions
		}
		seen := map[string]bool{}
		for _, id := range ids {
			if id == "" || len(id) > 191 || !utf8.ValidString(id) || seen[id] {
				return nil, ErrOptions
			}
			seen[id] = true
		}
		return seen, nil
	}
	profiles, err := validateIDs(profileIDs, 1000)
	if err != nil {
		return nil, err
	}
	providers, err := validateIDs(manualProviderIDs, 100)
	if err != nil {
		return nil, err
	}
	candidates, err := ReadCandidates(tx, selection)
	if err != nil {
		return nil, err
	}
	out := []ManualFlatQuote{}
	if len(profiles) == 0 || len(providers) == 0 {
		return out, nil
	}
	var active []string
	if tx.Table("fulfillment_shipping_profiles").Select("id").Where("id IN ? AND deleted_at IS NULL", profileIDs).Order("id").Limit(1001).Scan(&active).Error != nil || len(active) != len(profiles) {
		return nil, ErrOptions
	}
	ids := []string{}
	selected := []Candidate{}
	locationIDs := []string{}
	locations := map[string]bool{}
	for _, c := range candidates {
		if c.PriceType != "flat" || c.ProviderID == nil || !c.ProviderEnabled || !providers[*c.ProviderID] || c.ShippingProfileID == nil || !profiles[*c.ShippingProfileID] {
			continue
		}
		selected = append(selected, c)
		ids = append(ids, c.ID)
		for _, id := range c.LocationIDs {
			if !locations[id] {
				locations[id] = true
				locationIDs = append(locationIDs, id)
			}
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	var links []struct{ StockLocationID, FulfillmentProviderID string }
	if tx.Table("fulfillment_location_providers").Select("stock_location_id,fulfillment_provider_id").Where("stock_location_id IN ? AND fulfillment_provider_id IN ? AND deleted_at IS NULL", locationIDs, manualProviderIDs).Order("stock_location_id,fulfillment_provider_id").Limit(10001).Scan(&links).Error != nil || len(links) > 10000 {
		return nil, ErrOptions
	}
	type pair struct{ location, provider string }
	allowed := map[pair]bool{}
	for _, link := range links {
		allowed[pair{link.StockLocationID, link.FulfillmentProviderID}] = true
	}
	eligible := []Candidate{}
	ids = nil
	for _, c := range selected {
		filtered := []string{}
		for _, id := range c.LocationIDs {
			if allowed[pair{id, *c.ProviderID}] {
				filtered = append(filtered, id)
			}
		}
		if len(filtered) == 0 {
			continue
		}
		c.LocationIDs = filtered
		eligible = append(eligible, c)
		ids = append(ids, c.ID)
	}
	if len(ids) == 0 {
		return out, nil
	}
	prices, err := ReadFlatPrices(tx, ids, context, at)
	if err != nil {
		return nil, err
	}
	for _, c := range eligible {
		if price := prices[c.ID]; price != nil {
			out = append(out, ManualFlatQuote{Option: c, Price: *price})
		}
	}
	return out, nil
}
