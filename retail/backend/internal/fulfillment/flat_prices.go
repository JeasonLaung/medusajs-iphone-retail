package fulfillment

import (
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/pricing"
	"gorm.io/gorm"
)

// ReadFlatPrices prices server-resolved flat shipping options in the caller's
// transaction. This grants NO eligibility: channel/location/set/zone/address,
// profile, rules and provider checks must precede use as an offered option.
// The trusted coordinator supplies pricing context; no provider is called and
// no missing price becomes zero. Calculated, missing and deleted options have
// no quote. Multiple active price-set links are ambiguous and reject the batch.
func ReadFlatPrices(tx *gorm.DB, optionIDs []string, context pricing.Context, at time.Time) (map[string]*pricing.Quote, error) {
	if tx == nil || tx.Statement == nil || len(optionIDs) > 1000 || at.IsZero() {
		return nil, pricing.ErrInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, pricing.ErrInput
	}
	if err := Ready(tx.Statement.Context, tx); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, id := range optionIDs {
		if id == "" || len(id) > 191 || !utf8.ValidString(id) || seen[id] {
			return nil, pricing.ErrInput
		}
		seen[id] = true
	}
	out := map[string]*pricing.Quote{}
	if len(optionIDs) == 0 {
		return out, nil
	}
	var links []struct{ OptionID, PriceSetID string }
	if tx.Table("fulfillment_option_price_sets l").Select("l.shipping_option_id AS option_id,l.price_set_id").Joins("JOIN fulfillment_shipping_options o ON o.id=l.shipping_option_id AND o.deleted_at IS NULL AND o.price_type='flat'").Joins("JOIN pricing_price_sets ps ON ps.id=l.price_set_id AND ps.deleted_at IS NULL").Where("l.shipping_option_id IN ? AND l.deleted_at IS NULL", optionIDs).Order("l.shipping_option_id,l.price_set_id").Limit(1001).Scan(&links).Error != nil || len(links) > 1000 {
		return nil, pricing.ErrInput
	}
	optionSets := map[string]string{}
	setIDs := []string{}
	sets := map[string]bool{}
	for _, link := range links {
		if _, exists := optionSets[link.OptionID]; exists {
			return nil, pricing.ErrInput
		}
		optionSets[link.OptionID] = link.PriceSetID
		if !sets[link.PriceSetID] {
			sets[link.PriceSetID] = true
			setIDs = append(setIDs, link.PriceSetID)
		}
	}
	quotes, err := pricing.ReadSets(tx, setIDs, context, at)
	if err != nil {
		return nil, err
	}
	for option, set := range optionSets {
		out[option] = quotes[set]
	}
	return out, nil
}
