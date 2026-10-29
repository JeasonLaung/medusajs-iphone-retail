package cart

import (
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ManualShippingMethodInput carries only option identity and the manual
// provider's method data. Provider registration/amount/profile stay server-owned.
type ManualShippingMethodInput struct {
	OptionID string
	Data     json.RawMessage
}

type SavedShippingMethod struct{ ID, OptionID string }

// ReplaceManualShippingMethods is the persistence PHASE of a caller-owned
// transaction. It merges saved and incoming selections and checks stock, then
// soft-deletes methods
// colliding with incoming profiles (and unknown profiles), then inserts exact
// current prices. Caller MUST roll back after ANY error and complete refresh,
// durable idempotency/version/payment invalidation before committing. This
// helper does not commit, reserve stock or expose a public write capability.
func ReplaceManualShippingMethods(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string, input []ManualShippingMethodInput, manualProviderIDs []string, at time.Time) ([]SavedShippingMethod, error) {
	if len(input) > 100 {
		return nil, ErrInput
	}
	ids := []string{}
	data := map[string]string{}
	for _, method := range input {
		if !idPattern.MatchString(method.OptionID) {
			return nil, ErrInput
		}
		if _, ok := data[method.OptionID]; ok {
			return nil, ErrInput
		}
		raw := method.Data
		if len(raw) == 0 {
			raw = json.RawMessage(`{}`)
		}
		var object map[string]json.RawMessage
		if len(raw) > 65536 || !utf8.Valid(raw) || json.Unmarshal(raw, &object) != nil || object == nil {
			return nil, ErrInput
		}
		ids = append(ids, method.OptionID)
		data[method.OptionID] = string(raw)
	}
	quotes, err := ReadManualShippingQuotes(ctx, tx, store, ref, version, manualProviderIDs, at)
	if err != nil {
		return nil, err
	}
	profiles := map[string]bool{}
	currentProfiles := map[string]bool{}
	byOption := map[string]string{}
	for _, group := range quotes {
		currentProfiles[group.Group.ProfileID] = true
		for _, quote := range group.Quotes {
			byOption[quote.Option.ID] = group.Group.ProfileID
		}
	}
	for _, id := range ids {
		profile := byOption[id]
		if profile == "" || profiles[profile] {
			return nil, ErrItemUnavailable
		}
		profiles[profile] = true
	}
	var old []struct {
		ID        string
		ProfileID *string
		OptionID  *string
	}
	r := tx.WithContext(ctx).Table("cart_shipping_methods m").Select("m.id,p.id AS profile_id,o.id AS option_id").Joins("LEFT JOIN fulfillment_shipping_options o ON o.id=m.shipping_option_id AND o.deleted_at IS NULL").Joins("LEFT JOIN fulfillment_shipping_profiles p ON p.id=o.shipping_profile_id AND p.deleted_at IS NULL").Where("m.cart_id=? AND m.deleted_at IS NULL", ref.CartID).Order("m.id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(101).Scan(&old)
	if r.Error != nil || len(old) > 100 {
		return nil, ErrSchema
	}
	remove := []string{}
	retained := 0
	effective := append([]string(nil), ids...)
	for _, method := range old {
		if !idPattern.MatchString(method.ID) {
			return nil, ErrSchema
		}
		if method.ProfileID == nil || profiles[*method.ProfileID] {
			remove = append(remove, method.ID)
		} else {
			retained++
			if currentProfiles[*method.ProfileID] {
				if method.OptionID == nil {
					return nil, ErrSchema
				}
				effective = append(effective, *method.OptionID)
			}
		}
	}
	selection, err := readManualShippingSelection(ctx, tx, store, ref, version, effective, manualProviderIDs, at, false)
	if err != nil {
		return nil, err
	}
	if retained+len(input) > 100 {
		return nil, ErrSchema
	}
	type prepared struct {
		id, option, name, scalar, raw, data string
		inclusive                           bool
	}
	methods := []prepared{}
	for _, group := range selection.Groups {
		if !profiles[group.Group.ProfileID] {
			continue
		}
		q := group.Quote
		if !utf8.ValidString(q.Option.Name) || len(q.Option.Name) > 65536 {
			return nil, ErrSchema
		}
		scalar, raw, err := sqlAmount(q.Price.Amount)
		if err != nil {
			return nil, err
		}
		methods = append(methods, prepared{id: "casm_" + uuid.NewString(), option: q.Option.ID, name: q.Option.Name, scalar: scalar, raw: raw, data: data[q.Option.ID], inclusive: q.Price.CalculatedTaxInclusive})
	}
	if len(remove) > 0 {
		r := tx.Table("cart_shipping_methods").Where("cart_id=? AND id IN ? AND deleted_at IS NULL", ref.CartID, remove).Updates(map[string]any{"deleted_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
		if r.Error != nil || r.RowsAffected != int64(len(remove)) {
			return nil, ErrStorage
		}
	}
	out := []SavedShippingMethod{}
	for _, method := range methods {
		if tx.Exec(`INSERT INTO cart_shipping_methods(id,cart_id,name,amount,raw_amount,is_tax_inclusive,shipping_option_id,data,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, method.id, ref.CartID, method.name, method.scalar, method.raw, method.inclusive, method.option, method.data).Error != nil {
			return nil, ErrStorage
		}
		out = append(out, SavedShippingMethod{ID: method.id, OptionID: method.option})
	}
	return out, nil
}
