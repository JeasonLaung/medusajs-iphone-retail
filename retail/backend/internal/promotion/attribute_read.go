package promotion

import (
	"encoding/json"
	"sort"

	"gorm.io/gorm"
)

// contextAttribute follows the original explicit customer attribute projection.
// It does not infer account ownership from email, source IDs or metadata.
func contextAttribute(context map[string]any, attribute string) (string, error) {
	var value any
	switch attribute {
	case "customer_id":
		value = context["customer_id"]
		if value == nil {
			if customer, ok := context["customer"].(map[string]any); ok {
				value = customer["id"]
			}
		}
	case "customer_email":
		value = context["email"]
	default:
		return "", ErrCompute
	}
	text, ok := value.(string)
	if !ok || text == "" || !readText(text, 65535) {
		return "", ErrCompute
	}
	return text, nil
}

// ReadAttributeUsages reads only the exact attributes needed by candidates from
// ReadActive, using the same caller-owned snapshot and trusted cart context.
// Its return value contains private customer facts and must never be a public
// DTO, log payload, cache key or receipt. This is preview, not a locking check.
func ReadAttributeUsages(tx *gorm.DB, candidates []ActiveConfiguration, context json.RawMessage) (map[string]AttributeUsageSnapshot, error) {
	if tx == nil || tx.Statement == nil || len(candidates) > 1000 || len(context) > 1<<20 {
		return nil, ErrUsageInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, ErrUsageInput
	}
	if err := Ready(tx.Statement.Context, tx); err != nil {
		return nil, err
	}
	parsed, err := decode(context)
	if err != nil {
		return nil, ErrCompute
	}
	object, ok := parsed.(map[string]any)
	if !ok {
		return nil, ErrCompute
	}
	out := map[string]AttributeUsageSnapshot{}
	ids := []string{}
	for _, p := range candidates {
		b := p.Budget
		if p.Method == nil || b == nil || b.Type != "use_by_attribute" {
			continue
		}
		if !usageID.MatchString(b.ID) || b.Attribute == nil {
			return nil, ErrCompute
		}
		value, err := contextAttribute(object, *b.Attribute)
		if err != nil {
			return nil, err
		}
		if previous, present := out[b.ID]; present {
			if previous.Attribute != *b.Attribute || previous.Value != value {
				return nil, ErrCompute
			}
			continue
		}
		out[b.ID] = AttributeUsageSnapshot{Attribute: *b.Attribute, Value: value}
		ids = append(ids, b.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		fact := out[id]
		var rows []struct {
			ID, Used string
			RawUsed  string
		}
		// Full binary equality preserves case, trailing spaces and long values.
		// A second effective row is an unresolved schema hold, not permission
		// to choose whichever record happens to sort first.
		if tx.Table("promotion_budget_usages").Select("id,CAST(used AS CHAR) AS used,LEFT(raw_used,65537) AS raw_used").Where("budget_id=? AND deleted_at IS NULL AND BINARY attribute_value=BINARY ?", id, fact.Value).Order("id").Limit(2).Scan(&rows).Error != nil || len(rows) > 1 {
			return nil, ErrSchema
		}
		if len(rows) == 1 {
			row := rows[0]
			if !usageID.MatchString(row.ID) {
				return nil, ErrSchema
			}
			used, err := readAmount(&row.Used, &row.RawUsed)
			if err != nil {
				return nil, err
			}
			fact.Found, fact.Used = true, used
			out[id] = fact
		}
	}
	return out, nil
}
