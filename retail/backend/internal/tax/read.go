package tax

import (
	"regexp"
	"strings"

	"gorm.io/gorm"
)

var countryCode = regexp.MustCompile(`^[a-z]{2}$`)

type CalculationContext struct {
	Country, Province string
	Automatic, Force  bool
}

type CalculationItem struct {
	Item
	Giftcard bool
}

// ReadSystem uses the caller's transaction so tax configuration and other cart
// facts share one snapshot. It does not commit, write, or choose a provider for
// the caller. Only the configured and enabled built-in system is supported.
func ReadSystem(tx *gorm.DB, context CalculationContext, items []CalculationItem) ([]Line, error) {
	if tx == nil || tx.Statement == nil {
		return nil, ErrInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, ErrInput
	}
	if err := Ready(tx.Statement.Context, tx); err != nil {
		return nil, err
	}
	if len(items) > 1000 {
		return nil, ErrInput
	}
	seen := map[string]bool{}
	for _, item := range items {
		if !validItem(item.Item) || seen[item.ID] {
			return nil, ErrInput
		}
		seen[item.ID] = true
	}
	if !context.Automatic && !context.Force {
		return []Line{}, nil
	}
	if context.Country == "" {
		if context.Force {
			return nil, ErrInput
		}
		return []Line{}, nil
	}
	country, province := strings.ToLower(context.Country), strings.ToLower(context.Province)
	if !countryCode.MatchString(country) || !bounded(province, 191) {
		return nil, ErrInput
	}
	var regions []struct {
		ID                                 string
		CountryCode                        string
		ProvinceCode, ParentID, ProviderID *string
	}
	query := tx.Table("tax_regions").Select("id,LEFT(country_code,3) AS country_code,LEFT(province_code,192) AS province_code,parent_id,provider_id").Where("country_code=? AND deleted_at IS NULL", country)
	if province == "" {
		query = query.Where("province_code IS NULL")
	} else {
		query = query.Where("province_code IS NULL OR province_code=?", province)
	}
	if query.Order("id").Limit(3).Scan(&regions).Error != nil || len(regions) > 2 {
		return nil, ErrInput
	}
	var rootID, providerID string
	for _, region := range regions {
		if !id(region.ID) || region.CountryCode != country {
			return nil, ErrInput
		}
		if region.ProvinceCode == nil {
			if rootID != "" || region.ParentID != nil {
				return nil, ErrInput
			}
			rootID = region.ID
			if region.ProviderID != nil {
				providerID = *region.ProviderID
			}
		}
	}
	// The original module returns no lines when the country parent is missing.
	if rootID == "" {
		return []Line{}, nil
	}
	// The original loader registers tp_<identifier>; the provider emits its
	// own identifier (system) in tax lines. These are deliberately different.
	if providerID != "tp_system" {
		return nil, ErrInput
	}
	var provider struct{ Enabled bool }
	r := tx.Table("tax_providers").Select("is_enabled AS enabled").Where("id=? AND deleted_at IS NULL", providerID).Scan(&provider)
	if r.Error != nil || r.RowsAffected != 1 || !provider.Enabled {
		return nil, ErrInput
	}
	regionIDs := []string{}
	byRegion := map[string]Region{}
	provinceSeen := false
	for _, region := range regions {
		if region.ProvinceCode != nil {
			if provinceSeen || *region.ProvinceCode != province || region.ParentID == nil || *region.ParentID != rootID || region.ProviderID != nil {
				return nil, ErrInput
			}
			provinceSeen = true
		}
		regionIDs = append(regionIDs, region.ID)
		byRegion[region.ID] = Region{ID: region.ID, ProvinceCode: region.ProvinceCode, ParentID: region.ParentID}
	}
	var rates []Rate
	if tx.Table("tax_rates").Select("id,tax_region_id,rate,LEFT(name,16385) AS name,code,is_default,is_combinable").Where("tax_region_id IN ? AND deleted_at IS NULL", regionIDs).Order("created_at,id").Limit(10001).Scan(&rates).Error != nil || len(rates) > 10000 {
		return nil, ErrInput
	}
	if len(rates) == 0 {
		return []Line{}, nil
	}
	ids := []string{}
	positions := map[string]int{}
	for i := range rates {
		rates[i].Region = byRegion[rates[i].RegionID]
		rates[i].Rules = []Rule{}
		ids = append(ids, rates[i].ID)
		positions[rates[i].ID] = i
	}
	var rules []struct {
		RateID string
		Rule
	}
	if tx.Table("tax_rate_rules").Select("tax_rate_id AS rate_id,LEFT(reference,192) AS reference,reference_id").Where("tax_rate_id IN ? AND deleted_at IS NULL", ids).Order("id").Limit(20001).Scan(&rules).Error != nil || len(rules) > 20000 {
		return nil, ErrInput
	}
	for _, rule := range rules {
		pos, ok := positions[rule.RateID]
		if !ok {
			return nil, ErrInput
		}
		rates[pos].Rules = append(rates[pos].Rules, rule.Rule)
	}
	if !validRates(rates) {
		return nil, ErrInput
	}
	groups := []Group{}
	for _, item := range items {
		if !item.Shipping && item.Giftcard {
			continue
		}
		selected, err := Select(item.Item, rates, regionIDs)
		if err != nil {
			return nil, err
		}
		// The source query does not define ordering for equal-priority rates.
		// Do not turn our deterministic SQL order into a new tax policy.
		for _, winner := range selected {
			score, _ := priority(winner, item.Item)
			for _, candidate := range rates {
				other, match := priority(candidate, item.Item)
				if candidate.ID != winner.ID && candidate.RegionID == winner.RegionID && match && other == score {
					return nil, ErrInput
				}
			}
		}
		groups = append(groups, Group{Item: item.Item, Rates: selected})
	}
	return SystemLines(groups)
}
