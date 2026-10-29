package promotion

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrUsageInput = errors.New("invalid promotion usage input")
var ErrUsageUnavailable = errors.New("promotion unavailable for usage registration")
var ErrUsageLimit = errors.New("promotion usage limit exceeded")
var ErrUsageStorage = errors.New("promotion usage storage unavailable")

var usageID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,191}$`)
var usageCurrency = regexp.MustCompile(`^[a-z]{3,16}$`)

// UsageAction is a server-computed positive adjustment, never a coupon payload
// supplied by a browser. PromotionID is the resolved promotion identity.
type UsageAction struct{ PromotionID, Amount string }
type UsageRegistration struct {
	Currency string
	// Server-resolved customer/registration attributes. Values stay private.
	Attributes map[string]string
	At         time.Time
}
type UsageResult struct{ Promotions, Budgets, AttributeUsages int }

type usagePromotion struct {
	ID, Status string
	CampaignID *string
	Limit      *int64
	Used       int64
	DeletedAt  *time.Time
}
type usageCampaign struct {
	ID                          string
	StartsAt, EndsAt, DeletedAt *time.Time
}
type usageBudget struct {
	ID, Type, CampaignID, Used, RawUsed      string
	Limit, RawLimit, CurrencyCode, Attribute *string
}

func usageValue(s string, raw *string) (*big.Rat, error) {
	n := totals.Number{Decimal: s}
	if raw != nil {
		if len(*raw) > 65536 || !utf8.ValidString(*raw) {
			return nil, ErrUsageInput
		}
		if json.Unmarshal([]byte(*raw), &n.Raw) != nil || n.Raw == nil {
			return nil, ErrUsageInput
		}
	}
	value, err := totals.Resolve(n)
	if err != nil {
		return nil, ErrUsageInput
	}
	r, ok := new(big.Rat).SetString(value)
	if !ok || r.Sign() < 0 {
		return nil, ErrUsageInput
	}
	return r, nil
}

func usageSQL(r *big.Rat) (string, string, error) {
	// Input decimal bounds permit at most 128 fractional places. Addition
	// introduces no new denominator, so this conversion is exact.
	s := strings.TrimRight(strings.TrimRight(r.FloatString(256), "0"), ".")
	if s == "" {
		s = "0"
	}
	canonical, err := totals.Resolve(totals.Number{Decimal: s})
	if err != nil || canonical != s || r.Sign() < 0 {
		return "", "", ErrUsageInput
	}
	parts := strings.SplitN(s, ".", 2)
	if len(parts[0]) > 20 {
		return "", "", ErrUsageInput
	}
	scalar := parts[0]
	if len(parts) == 2 {
		scalar += "." + parts[1][:min(18, len(parts[1]))]
	}
	raw, err := json.Marshal(totals.RawValue{Value: s, Precision: 100})
	if err != nil {
		return "", "", ErrUsageInput
	}
	return scalar, string(raw), nil
}

// RegisterUsage is a PHASE in the caller's transaction. It neither commits nor
// supplies durable idempotency. Caller MUST roll back on any error and bind
// success to the checkout/order operation receipt before committing. Do not
// invoke it when merely previewing/recalculating a draft shopping cart.
// It rechecks active configuration and serialized limits, not qualification.
func RegisterUsage(ctx context.Context, tx *gorm.DB, actions []UsageAction, in UsageRegistration) (UsageResult, error) {
	empty := UsageResult{}
	if tx == nil || tx.Statement == nil || len(actions) > 1000 || in.At.IsZero() || !usageCurrency.MatchString(in.Currency) || len(in.Attributes) > 100 {
		return empty, ErrUsageInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return empty, ErrUsageInput
	}
	for k, v := range in.Attributes {
		if k == "" || len(k) > 191 || len(v) > 65536 || !utf8.ValidString(k) || !utf8.ValidString(v) {
			return empty, ErrUsageInput
		}
	}
	amounts := map[string]*big.Rat{}
	for _, action := range actions {
		if !usageID.MatchString(action.PromotionID) {
			return empty, ErrUsageInput
		}
		amount, err := usageValue(action.Amount, nil)
		if err != nil || amount.Sign() <= 0 {
			return empty, ErrUsageInput
		}
		if old, ok := amounts[action.PromotionID]; ok {
			old.Add(old, amount)
		} else {
			amounts[action.PromotionID] = amount
		}
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx = tx.WithContext(bounded)
	if Ready(bounded, tx.Clauses(clause.Locking{Strength: "SHARE"})) != nil {
		return empty, ErrSchema
	}
	if len(actions) == 0 {
		return empty, nil
	}
	ids := []string{}
	for id := range amounts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	promotions := []usagePromotion{}
	campaignIDs := map[string]bool{}
	for _, id := range ids {
		var p usagePromotion
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("promotions").Select("id,status,campaign_id,`limit`,used,deleted_at").Where("id=?", id).Take(&p).Error != nil {
			return empty, ErrUsageUnavailable
		}
		if p.DeletedAt != nil || p.Status != "active" || p.Used < 0 || p.Limit != nil && (*p.Limit < 0 || p.Used >= *p.Limit) {
			return empty, ErrUsageLimit
		}
		promotions = append(promotions, p)
		if p.CampaignID != nil {
			campaignIDs[*p.CampaignID] = true
		}
	}
	campaignOrder := []string{}
	for id := range campaignIDs {
		campaignOrder = append(campaignOrder, id)
	}
	sort.Strings(campaignOrder)
	// Campaign locks also serialize configuration edits and discovery of a
	// campaign budget. Current locking reads ignore an older RR snapshot.
	for _, id := range campaignOrder {
		var c usageCampaign
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("promotion_campaigns").Select("id,starts_at,ends_at,deleted_at").Where("id=?", id).Take(&c).Error != nil {
			return empty, ErrUsageUnavailable
		}
		if c.DeletedAt != nil || c.StartsAt != nil && in.At.Before(*c.StartsAt) || c.EndsAt != nil && !in.At.Before(*c.EndsAt) {
			return empty, ErrUsageUnavailable
		}
	}
	// Discover budgets with a current read, then acquire their rows in ID order.
	var discovered []struct{ ID string }
	if len(campaignOrder) > 0 && tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("promotion_campaign_budgets").Select("id").Where("campaign_id IN ? AND deleted_at IS NULL", campaignOrder).Order("id").Limit(1001).Find(&discovered).Error != nil {
		return empty, ErrUsageStorage
	}
	if len(discovered) > 1000 {
		return empty, ErrUsageUnavailable
	}
	budgets := map[string]usageBudget{}
	for _, row := range discovered {
		var b usageBudget
		if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("promotion_campaign_budgets").Select("id,type,campaign_id,CAST(`limit` AS CHAR) AS `limit`,raw_limit,CAST(used AS CHAR) AS used,raw_used,currency_code,attribute").Where("id=? AND deleted_at IS NULL", row.ID).Take(&b).Error != nil {
			return empty, ErrUsageStorage
		}
		if _, ok := budgets[b.CampaignID]; ok {
			return empty, ErrUsageUnavailable
		}
		budgets[b.CampaignID] = b
	}
	type update struct {
		budget      usageBudget
		used, limit *big.Rat
		increment   *big.Rat
		attribute   string
		codes       int64
	}
	changes := map[string]*update{}
	for _, p := range promotions {
		if p.CampaignID == nil {
			continue
		}
		b, ok := budgets[*p.CampaignID]
		if !ok {
			continue
		}
		u, ok := changes[b.ID]
		if !ok {
			used, err := usageValue(b.Used, &b.RawUsed)
			if err != nil {
				return empty, ErrUsageUnavailable
			}
			u = &update{budget: b, used: used, increment: new(big.Rat)}
			if b.Limit != nil || b.RawLimit != nil {
				s := "0"
				if b.Limit != nil {
					s = *b.Limit
				}
				u.limit, err = usageValue(s, b.RawLimit)
				if err != nil {
					return empty, ErrUsageUnavailable
				}
			}
			changes[b.ID] = u
		}
		switch b.Type {
		case "spend":
			if b.CurrencyCode == nil || *b.CurrencyCode != in.Currency {
				return empty, ErrUsageUnavailable
			}
			u.increment.Add(u.increment, amounts[p.ID])
		case "usage":
			u.increment.Add(u.increment, big.NewRat(1, 1))
		case "use_by_attribute":
			if b.Attribute == nil || in.Attributes[*b.Attribute] == "" {
				return empty, ErrUsageInput
			}
			u.attribute = in.Attributes[*b.Attribute]
			u.codes++
			u.increment.Add(u.increment, big.NewRat(1, 1))
		default:
			return empty, ErrUsageUnavailable
		}
	}
	budgetOrder := []string{}
	for id := range changes {
		budgetOrder = append(budgetOrder, id)
	}
	sort.Strings(budgetOrder)
	// Prepare every change before touching durable counters.
	type write struct {
		table, id, scalar, raw string
		budgetID               string
		attribute              string
		fresh                  bool
	}
	writes := []write{}
	for _, id := range budgetOrder {
		u := changes[id]
		next := new(big.Rat).Add(u.used, u.increment)
		if u.budget.Type != "use_by_attribute" && u.limit != nil && next.Cmp(u.limit) > 0 {
			return empty, ErrUsageLimit
		}
		if u.budget.Type == "use_by_attribute" {
			var rows []struct{ ID, Used, RawUsed string }
			if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("promotion_budget_usages").Select("id,CAST(used AS CHAR) AS used,raw_used").Where("budget_id=? AND BINARY attribute_value=BINARY ? AND deleted_at IS NULL", id, u.attribute).Order("id").Limit(2).Find(&rows).Error != nil {
				return empty, ErrUsageStorage
			}
			if len(rows) > 1 {
				return empty, ErrUsageUnavailable
			}
			used := new(big.Rat)
			usageID := "pbu_" + uuid.NewString()
			if len(rows) == 1 {
				var err error
				usageID = rows[0].ID
				used, err = usageValue(rows[0].Used, &rows[0].RawUsed)
				if err != nil {
					return empty, ErrUsageUnavailable
				}
			}
			attributeNext := new(big.Rat).Add(used, big.NewRat(u.codes, 1))
			if u.limit != nil && attributeNext.Cmp(u.limit) > 0 {
				return empty, ErrUsageLimit
			}
			scalar, raw, err := usageSQL(attributeNext)
			if err != nil {
				return empty, err
			}
			writes = append(writes, write{table: "promotion_budget_usages", id: usageID, scalar: scalar, raw: raw, attribute: u.attribute, fresh: len(rows) == 0, budgetID: id})
		}
		scalar, raw, err := usageSQL(next)
		if err != nil {
			return empty, err
		}
		writes = append(writes, write{table: "promotion_campaign_budgets", id: id, scalar: scalar, raw: raw})
	}
	result := UsageResult{Promotions: len(promotions), Budgets: len(changes)}
	for _, p := range promotions {
		if p.Limit == nil {
			continue
		}
		r := tx.Exec("UPDATE promotions SET used=used+1,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND used=? AND deleted_at IS NULL", p.ID, p.Used)
		if r.Error != nil || r.RowsAffected != 1 {
			return empty, ErrUsageStorage
		}
	}
	for _, w := range writes {
		if w.fresh {
			if tx.Exec("INSERT INTO promotion_budget_usages(id,attribute_value,used,budget_id,raw_used,created_at,updated_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", w.id, w.attribute, w.scalar, w.budgetID, w.raw).Error != nil {
				return empty, ErrUsageStorage
			}
			result.AttributeUsages++
			continue
		}
		r := tx.Table(w.table).Where("id=? AND deleted_at IS NULL", w.id).Updates(map[string]any{"used": w.scalar, "raw_used": w.raw, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
		if r.Error != nil || r.RowsAffected != 1 {
			return empty, ErrUsageStorage
		}
		if w.table == "promotion_budget_usages" {
			result.AttributeUsages++
		}
	}
	return result, nil
}
