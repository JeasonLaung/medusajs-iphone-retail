package pricing

import (
	"sort"
	"time"

	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
)

// Read uses the caller's repeatable-read transaction. Both shared price sets and
// all applicable rules/lists are loaded before selection, never page-truncated.
func Read(tx *gorm.DB, variants []string, c Context, now time.Time) (map[string]*Quote, error) {
	quotes := map[string]*Quote{}
	if len(variants) > 10000 {
		return nil, ErrInput
	}
	if len(variants) == 0 {
		return quotes, nil
	}
	var links []struct{ VariantID, PriceSetID string }
	if err := tx.Table("catalog_variant_price_sets vp").Select("DISTINCT vp.variant_id,vp.price_set_id").Joins("JOIN pricing_price_sets ps ON ps.id=vp.price_set_id AND ps.deleted_at IS NULL").Where("vp.variant_id IN ? AND vp.deleted_at IS NULL", variants).Order("vp.variant_id,vp.price_set_id").Limit(10001).Scan(&links).Error; err != nil {
		return nil, err
	}
	if len(links) > 10000 {
		return nil, ErrInput
	}
	sets := map[string]bool{}
	variantSets := map[string]string{}
	for _, l := range links {
		if _, ok := variantSets[l.VariantID]; ok {
			return nil, ErrInput
		}
		variantSets[l.VariantID] = l.PriceSetID
		sets[l.PriceSetID] = true
	}
	if len(sets) == 0 {
		return quotes, nil
	}
	setIDs := []string{}
	for id := range sets {
		setIDs = append(setIDs, id)
	}
	sort.Strings(setIDs)
	selected, err := ReadSets(tx, setIDs, c, now)
	if err != nil {
		return nil, err
	}
	for id, set := range variantSets {
		quotes[id] = selected[set]
	}
	return quotes, nil
}

// ReadSets selects prices for trusted server-resolved price sets. It grants no
// product or shipping eligibility. Caller owns the consistent SQL snapshot.
// Missing/deleted sets produce no quote, never a fabricated zero amount.
func ReadSets(tx *gorm.DB, setIDs []string, c Context, now time.Time) (map[string]*Quote, error) {
	quotes := map[string]*Quote{}
	if tx == nil || len(setIDs) > 10000 || now.IsZero() {
		return nil, ErrInput
	}
	seen := map[string]bool{}
	for _, id := range setIDs {
		if id == "" || len(id) > 191 || seen[id] {
			return nil, ErrInput
		}
		seen[id] = true
	}
	if len(setIDs) == 0 {
		return quotes, nil
	}
	var active []string
	if tx.Table("pricing_price_sets").Select("id").Where("id IN ? AND deleted_at IS NULL", setIDs).Order("id").Limit(10001).Scan(&active).Error != nil || len(active) > 10000 {
		return nil, ErrInput
	}
	setIDs = active
	if len(setIDs) == 0 {
		return quotes, nil
	}
	var rows []struct {
		ID, PriceSetID, CurrencyCode, Amount, RawAmount                       string
		MinQuantity, MaxQuantity, RawMinQuantity, RawMaxQuantity, PriceListID *string
		RulesCount                                                            *int
	}
	budget := 20000
	if err := tx.Table("pricing_prices").Select("id,price_set_id,currency_code,amount,raw_amount,min_quantity,max_quantity,raw_min_quantity,raw_max_quantity,price_list_id,rules_count").Where("price_set_id IN ? AND currency_code=? AND deleted_at IS NULL", setIDs, c.Currency).Order("id").Limit(budget + 1).Scan(&rows).Error; err != nil {
		return nil, err
	}
	budget -= len(rows)
	if budget < 0 {
		return nil, ErrInput
	}
	if len(rows) == 0 {
		return quotes, nil
	}
	prices := make([]Price, 0, len(rows))
	positions := map[string]int{}
	ids := []string{}
	listSet := map[string]bool{}
	for _, r := range rows {
		amount, err := sourceNumber(r.Amount, &r.RawAmount)
		if err != nil {
			return nil, err
		}
		p := Price{ID: r.ID, SetID: r.PriceSetID, Currency: r.CurrencyCode, Amount: amount, ListID: r.PriceListID, RulesCount: r.RulesCount, Rules: []Rule{}}
		bound := func(value, raw *string) (*totals.Number, error) {
			if value == nil {
				if raw != nil {
					return nil, ErrInput
				}
				return nil, nil
			}
			n, e := sourceNumber(*value, raw)
			return &n, e
		}
		if p.Min, err = bound(r.MinQuantity, r.RawMinQuantity); err != nil {
			return nil, err
		}
		if p.Max, err = bound(r.MaxQuantity, r.RawMaxQuantity); err != nil {
			return nil, err
		}
		positions[p.ID] = len(prices)
		prices = append(prices, p)
		ids = append(ids, p.ID)
		if p.ListID != nil {
			listSet[*p.ListID] = true
		}
	}
	var rules []struct{ PriceID, Attribute, Operator, Value string }
	if err := tx.Table("pricing_price_rules").Select("price_id,attribute,operator,value").Where("price_id IN ? AND deleted_at IS NULL", ids).Order("id").Limit(budget + 1).Scan(&rules).Error; err != nil {
		return nil, err
	}
	budget -= len(rules)
	if budget < 0 {
		return nil, ErrInput
	}
	for _, r := range rules {
		p := &prices[positions[r.PriceID]]
		p.Rules = append(p.Rules, Rule{r.Attribute, r.Operator, r.Value})
	}
	lists := map[string]*List{}
	if len(listSet) > 0 {
		listIDs := []string{}
		for id := range listSet {
			listIDs = append(listIDs, id)
		}
		sort.Strings(listIDs)
		var records []struct {
			ID, Type, Status string
			StartsAt, EndsAt *time.Time
			RulesCount       *int
		}
		if err := tx.Table("pricing_price_lists").Select("id,type,status,starts_at,ends_at,rules_count").Where("id IN ? AND deleted_at IS NULL", listIDs).Order("id").Limit(budget + 1).Scan(&records).Error; err != nil {
			return nil, err
		}
		budget -= len(records)
		if budget < 0 {
			return nil, ErrInput
		}
		activeIDs := []string{}
		for _, r := range records {
			lists[r.ID] = &List{ID: r.ID, Type: r.Type, Status: r.Status, StartsAt: r.StartsAt, EndsAt: r.EndsAt, RulesCount: r.RulesCount, Rules: []ListRule{}}
			activeIDs = append(activeIDs, r.ID)
		}
		if len(activeIDs) > 0 {
			var listRules []struct {
				PriceListID, Attribute string
				Value                  *string
			}
			if err := tx.Table("pricing_price_list_rules").Select("price_list_id,attribute,value").Where("price_list_id IN ? AND deleted_at IS NULL", activeIDs).Order("id").Limit(budget + 1).Scan(&listRules).Error; err != nil {
				return nil, err
			}
			budget -= len(listRules)
			if budget < 0 {
				return nil, ErrInput
			}
			for _, r := range listRules {
				l := lists[r.PriceListID]
				l.Rules = append(l.Rules, ListRule{r.Attribute, r.Value})
			}
		}
	}
	bySet := map[string][]Price{}
	for i := range prices {
		p := &prices[i]
		if p.ListID != nil {
			p.List = lists[*p.ListID]
		}
		bySet[p.SetID] = append(bySet[p.SetID], *p)
	}
	prefs := []Preference{}
	if err := tx.Table("pricing_preferences").Select("attribute,value,is_tax_inclusive").Where("deleted_at IS NULL AND ((attribute='currency_code' AND value=?) OR (attribute='region_id' AND value=?))", c.Currency, c.Region).Order("id").Limit(101).Scan(&prefs).Error; err != nil {
		return nil, err
	}
	if len(prefs) > 100 {
		return nil, ErrInput
	}
	selected := map[string]*Quote{}
	for _, id := range setIDs {
		q, err := Select(bySet[id], prefs, c, now)
		if err != nil {
			return nil, err
		}
		selected[id] = q
	}
	return selected, nil
}
