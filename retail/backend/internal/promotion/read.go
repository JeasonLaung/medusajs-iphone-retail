package promotion

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
)

// ActiveConfiguration is an internal server projection, not a public DTO or a
// qualification result. In particular it contains no customer attribute usages.
type ActiveConfiguration struct {
	ID, Code, Type          string
	Automatic, TaxInclusive bool
	Limit                   *int64
	Used                    int64
	CampaignID              *string
	Method                  *ApplicationMethod
	Budget                  *CampaignBudget
	Rules                   []Rule
}
type ApplicationMethod struct {
	ID, Type, Target, Value                string
	Allocation, Currency                   *string
	MaxQuantity, ApplyQuantity, MinimumBuy *int64
	BuyRules, TargetRules                  []Rule
}
type CampaignBudget struct {
	ID, Type, Used             string
	Limit, Currency, Attribute *string
}
type ActiveRequest struct {
	Codes            []string
	IncludeAutomatic bool
	At               time.Time
}

func readText(s string, limit int) bool { return len(s) <= limit && utf8.ValidString(s) }
func readAmount(s, raw *string) (string, error) {
	n := totals.Number{Decimal: "0"}
	if s != nil {
		n.Decimal = *s
	}
	if raw != nil {
		if !readText(*raw, 65536) || json.Unmarshal([]byte(*raw), &n.Raw) != nil || n.Raw == nil {
			return "", ErrSchema
		}
	}
	v, err := totals.Resolve(n)
	if err != nil {
		return "", ErrSchema
	}
	r, ok := decimal(v)
	if !ok || r.Sign() < 0 {
		return "", ErrSchema
	}
	return v, nil
}

// ReadActive reads the active requested codes and optionally automatic
// promotions in the caller's transaction, sharing its snapshot with cart facts.
// Starts are inclusive, ends exclusive. Unknown/inactive codes are absent, not
// fabricated coupons. It does not consume budgets or evaluate order/item rules.
func ReadActive(tx *gorm.DB, in ActiveRequest) ([]ActiveConfiguration, error) {
	if tx == nil || tx.Statement == nil || in.At.IsZero() || len(in.Codes) > 100 {
		return nil, ErrUsageInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, ErrUsageInput
	}
	codes := []string{}
	seenCodes := map[string]bool{}
	for _, code := range in.Codes {
		if code == "" || !readText(code, 191) {
			return nil, ErrUsageInput
		}
		if !seenCodes[code] {
			codes = append(codes, code)
			seenCodes[code] = true
		}
	}
	if err := Ready(tx.Statement.Context, tx); err != nil {
		return nil, err
	}
	out := []ActiveConfiguration{}
	if len(codes) == 0 && !in.IncludeAutomatic {
		return out, nil
	}
	var rows []struct {
		ID, Code, Type              string
		IsAutomatic, IsTaxInclusive bool
		Limit                       *int64
		Used                        int64
		CampaignID                  *string
	}
	query := tx.Table("promotions p").Select("p.id,p.code,p.type,p.is_automatic,p.is_tax_inclusive,p.`limit`,p.used,p.campaign_id").Joins("LEFT JOIN promotion_campaigns c ON c.id=p.campaign_id AND c.deleted_at IS NULL").Where("p.deleted_at IS NULL AND p.status='active'").Where("p.campaign_id IS NULL OR (c.id IS NOT NULL AND (c.starts_at IS NULL OR c.starts_at<=?) AND (c.ends_at IS NULL OR c.ends_at>?))", in.At, in.At)
	if in.IncludeAutomatic {
		query = query.Where("p.code IN ? OR p.is_automatic=TRUE", codes)
	} else {
		query = query.Where("p.code IN ?", codes)
	}
	if query.Order("p.id").Limit(1001).Scan(&rows).Error != nil || len(rows) > 1000 {
		return nil, ErrSchema
	}
	ids, campaignIDs := []string{}, []string{}
	byID := map[string]int{}
	seenCampaign := map[string]bool{}
	activeCodes := map[string]bool{}
	for _, row := range rows {
		if !usageID.MatchString(row.ID) || row.Code == "" || !readText(row.Code, 191) || activeCodes[row.Code] || (row.Type != "standard" && row.Type != "buyget") || row.Used < 0 || row.Limit != nil && *row.Limit < 0 {
			return nil, ErrSchema
		}
		activeCodes[row.Code] = true
		byID[row.ID] = len(out)
		ids = append(ids, row.ID)
		out = append(out, ActiveConfiguration{ID: row.ID, Code: row.Code, Type: row.Type, Automatic: row.IsAutomatic, TaxInclusive: row.IsTaxInclusive, Limit: row.Limit, Used: row.Used, CampaignID: row.CampaignID, Rules: []Rule{}})
		if row.CampaignID != nil {
			if !usageID.MatchString(*row.CampaignID) {
				return nil, ErrSchema
			}
			if !seenCampaign[*row.CampaignID] {
				campaignIDs = append(campaignIDs, *row.CampaignID)
				seenCampaign[*row.CampaignID] = true
			}
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	var methods []struct {
		ID, PromotionID, Type, TargetType                 string
		Value, RawValue, Allocation, CurrencyCode         *string
		MaxQuantity, ApplyToQuantity, BuyRulesMinQuantity *int64
	}
	if tx.Table("promotion_application_methods").Select("id,promotion_id,type,target_type,CAST(value AS CHAR) AS value,raw_value,allocation,currency_code,max_quantity,apply_to_quantity,buy_rules_min_quantity").Where("promotion_id IN ? AND deleted_at IS NULL", ids).Order("id").Limit(1001).Scan(&methods).Error != nil || len(methods) > 1000 {
		return nil, ErrSchema
	}
	methodIDs := []string{}
	byMethod := map[string]*ApplicationMethod{}
	for _, row := range methods {
		index, ok := byID[row.PromotionID]
		if !ok || out[index].Method != nil || !usageID.MatchString(row.ID) || (row.Type != "fixed" && row.Type != "percentage") || (row.TargetType != "items" && row.TargetType != "order" && row.TargetType != "shipping_methods") {
			return nil, ErrSchema
		}
		if row.Allocation != nil && *row.Allocation != "each" && *row.Allocation != "once" && *row.Allocation != "across" {
			return nil, ErrSchema
		}
		if row.CurrencyCode != nil && !usageCurrency.MatchString(*row.CurrencyCode) {
			return nil, ErrSchema
		}
		for _, n := range []*int64{row.MaxQuantity, row.ApplyToQuantity, row.BuyRulesMinQuantity} {
			if n != nil && (*n < 0 || *n > 2147483647) {
				return nil, ErrSchema
			}
		}
		value, err := readAmount(row.Value, row.RawValue)
		if err != nil {
			return nil, err
		}
		method := &ApplicationMethod{ID: row.ID, Type: row.Type, Target: row.TargetType, Value: value, Allocation: row.Allocation, Currency: row.CurrencyCode, MaxQuantity: row.MaxQuantity, ApplyQuantity: row.ApplyToQuantity, MinimumBuy: row.BuyRulesMinQuantity, BuyRules: []Rule{}, TargetRules: []Rule{}}
		out[index].Method = method
		byMethod[row.ID] = method
		methodIDs = append(methodIDs, row.ID)
	}
	if len(campaignIDs) > 0 {
		var budgets []struct {
			ID, CampaignID, Type, Used, RawUsed      string
			Limit, RawLimit, CurrencyCode, Attribute *string
		}
		if tx.Table("promotion_campaign_budgets").Select("id,campaign_id,type,CAST(used AS CHAR) AS used,raw_used,CAST(`limit` AS CHAR) AS `limit`,raw_limit,currency_code,attribute").Where("campaign_id IN ? AND deleted_at IS NULL", campaignIDs).Order("id").Limit(1001).Scan(&budgets).Error != nil || len(budgets) > 1000 {
			return nil, ErrSchema
		}
		byCampaign := map[string]*CampaignBudget{}
		for _, row := range budgets {
			if byCampaign[row.CampaignID] != nil || !usageID.MatchString(row.ID) {
				return nil, ErrSchema
			}
			switch row.Type {
			case "spend", "usage", "use_by_attribute", "spend_by_attribute":
			default:
				return nil, ErrSchema
			}
			if row.CurrencyCode != nil && !usageCurrency.MatchString(*row.CurrencyCode) || row.Attribute != nil && !readText(*row.Attribute, 191) {
				return nil, ErrSchema
			}
			used, err := readAmount(&row.Used, &row.RawUsed)
			if err != nil {
				return nil, err
			}
			budget := &CampaignBudget{ID: row.ID, Type: row.Type, Used: used, Currency: row.CurrencyCode, Attribute: row.Attribute}
			if row.Limit != nil || row.RawLimit != nil {
				limit, err := readAmount(row.Limit, row.RawLimit)
				if err != nil {
					return nil, err
				}
				budget.Limit = &limit
			}
			byCampaign[row.CampaignID] = budget
		}
		for i := range out {
			if out[i].CampaignID != nil {
				out[i].Budget = byCampaign[*out[i].CampaignID]
			}
		}
	}
	// Read links first so a soft-deleted rule never silently turns a
	// nonempty restriction into an unrestricted promotion.
	type link struct{ OwnerID, RuleID string }
	links := map[string][]link{}
	ruleIDs := []string{}
	seenRule := map[string]bool{}
	for _, spec := range []struct {
		table, owner string
		ids          []string
	}{{"promotion_order_rules", "promotion_id", ids}, {"promotion_buy_rules", "application_method_id", methodIDs}, {"promotion_target_rules", "application_method_id", methodIDs}} {
		if len(spec.ids) == 0 {
			continue
		}
		var selected []link
		if tx.Table(spec.table).Select(spec.owner+" AS owner_id,promotion_rule_id AS rule_id").Where(spec.owner+" IN ?", spec.ids).Order(spec.owner+",promotion_rule_id").Limit(5001).Scan(&selected).Error != nil || len(selected) > 5000 {
			return nil, ErrSchema
		}
		links[spec.table] = selected
		for _, l := range selected {
			if !usageID.MatchString(l.RuleID) {
				return nil, ErrSchema
			}
			if !seenRule[l.RuleID] {
				ruleIDs = append(ruleIDs, l.RuleID)
				seenRule[l.RuleID] = true
			}
		}
	}
	if len(ruleIDs) > 1000 {
		return nil, ErrSchema
	}
	if len(ruleIDs) > 0 {
		var selected []struct{ ID, Attribute, Operator string }
		if tx.Table("promotion_rules").Select("id,LEFT(attribute,192) AS attribute,operator").Where("id IN ? AND deleted_at IS NULL", ruleIDs).Order("id").Limit(1001).Scan(&selected).Error != nil || len(selected) != len(ruleIDs) {
			return nil, ErrSchema
		}
		rules := map[string]*Rule{}
		for _, row := range selected {
			if row.Attribute == "" || !readText(row.Attribute, 191) {
				return nil, ErrSchema
			}
			rules[row.ID] = &Rule{Attribute: row.Attribute, Operator: row.Operator, Values: []RuleValue{}}
		}
		var values []struct{ ID, PromotionRuleID, Value string }
		if tx.Table("promotion_rule_values").Select("id,promotion_rule_id,LEFT(value,65537) AS value").Where("promotion_rule_id IN ? AND deleted_at IS NULL", ruleIDs).Order("id").Limit(10001).Scan(&values).Error != nil || len(values) > 10000 {
			return nil, ErrSchema
		}
		for _, v := range values {
			rule := rules[v.PromotionRuleID]
			if rule == nil || !readText(v.Value, 65536) || len(rule.Values) >= 1000 {
				return nil, ErrSchema
			}
			encoded, err := json.Marshal(v.Value)
			if err != nil || len(encoded) > 65536 {
				return nil, ErrSchema
			}
			rule.Values = append(rule.Values, RuleValue{Value: encoded})
		}

		for _, rule := range rules {
			if len(strings.Split(rule.Attribute, ".")) > 32 || len(rule.Values) == 0 {
				return nil, ErrSchema
			}
			switch rule.Operator {
			case "eq", "in", "ne", "gt", "gte", "lt", "lte":
			default:
				return nil, ErrSchema
			}
			if rule.Operator == "gt" || rule.Operator == "gte" || rule.Operator == "lt" || rule.Operator == "lte" {
				for _, v := range rule.Values {
					var text string
					if json.Unmarshal(v.Value, &text) != nil {
						return nil, ErrSchema
					}
					if _, ok := decimal(text); !ok {
						return nil, ErrSchema
					}
				}
			}
		}
		copyRule := func(id string) Rule {
			source := rules[id]
			out := Rule{Attribute: source.Attribute, Operator: source.Operator, Values: make([]RuleValue, len(source.Values))}
			for i, v := range source.Values {
				out.Values[i].Value = append(json.RawMessage(nil), v.Value...)
			}
			return out
		}
		for _, l := range links["promotion_order_rules"] {
			index, ok := byID[l.OwnerID]
			if !ok {
				return nil, ErrSchema
			}
			out[index].Rules = append(out[index].Rules, copyRule(l.RuleID))
		}
		for _, spec := range []struct {
			table string
			buy   bool
		}{{"promotion_buy_rules", true}, {"promotion_target_rules", false}} {
			for _, l := range links[spec.table] {
				method := byMethod[l.OwnerID]
				if method == nil {
					return nil, ErrSchema
				}
				if spec.buy {
					method.BuyRules = append(method.BuyRules, copyRule(l.RuleID))
				} else {
					method.TargetRules = append(method.TargetRules, copyRule(l.RuleID))
				}
			}
		}
	}
	// Original database selection orders application value descending. Full
	// buy-get priority and qualification remain the caller's next phase.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Method == nil {
			return false
		}
		if out[j].Method == nil {
			return true
		}
		a, _ := decimal(out[i].Method.Value)
		b, _ := decimal(out[j].Method.Value)
		return a.Cmp(b) > 0
	})
	return out, nil
}
