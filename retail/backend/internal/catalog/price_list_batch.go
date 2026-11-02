package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// One batch preserves the identity of updated prices and commits all three
// source actions together.
type PriceListPriceRef struct {
	ID        string `json:"id"`
	ProductID string `json:"product_id"`
	VariantID string `json:"variant_id"`
}
type PriceListPriceUpdate struct {
	PriceListPriceRef
	Amount      string `json:"amount"`
	MinQuantity string `json:"min_quantity"`
	MaxQuantity string `json:"max_quantity"`
}
type PriceListBatchInput struct {
	OperationID      string                 `json:"operation_id"`
	ExpectedRevision string                 `json:"expected_revision"`
	ProductIDs       []string               `json:"product_ids"`
	Create           []InitialListPrice     `json:"create"`
	Update           []PriceListPriceUpdate `json:"update"`
	Delete           []PriceListPriceRef    `json:"delete"`
}

func validPriceListBatch(in PriceListBatchInput) bool {
	if !validPriceListAdd(PriceListAddInput{in.OperationID, in.ExpectedRevision, in.ProductIDs, in.Create}) || in.Update == nil || in.Delete == nil || len(in.Create)+len(in.Update)+len(in.Delete) == 0 || len(in.Create)+len(in.Update)+len(in.Delete) > 200 {
		return false
	}
	products := map[string]bool{}
	for _, id := range in.ProductIDs {
		products[id] = true
	}
	seen := map[string]bool{}
	ref := func(p PriceListPriceRef) bool {
		if !validPricePath(p.ProductID, p.VariantID, p.ID) || !products[p.ProductID] || seen[p.ID] {
			return false
		}
		seen[p.ID] = true
		return true
	}
	for _, p := range in.Update {
		if !ref(p.PriceListPriceRef) || !validPriceAmount(PriceAmountInput{in.OperationID, in.ExpectedRevision, p.Amount}) || !validPriceQuantities(p.MinQuantity, p.MaxQuantity) {
			return false
		}
	}
	for _, p := range in.Delete {
		if !ref(p) {
			return false
		}
	}
	return true
}
func decodePriceListBatch(r io.Reader) (PriceListBatchInput, error) {
	var out PriceListBatchInput
	raw, e := io.ReadAll(io.LimitReader(r, 262145))
	if e != nil || len(raw) > 262144 || !utf8.Valid(raw) {
		return out, ErrPriceInput
	}
	f, e := listCreateObject(raw, []string{"operation_id", "expected_revision", "product_ids", "create", "update", "delete"})
	if e != nil {
		return out, e
	}
	addRaw, _ := json.Marshal(map[string]json.RawMessage{"operation_id": f["operation_id"], "expected_revision": f["expected_revision"], "product_ids": f["product_ids"], "prices": f["create"]})
	add, e := decodePriceListAdd(bytes.NewReader(addRaw))
	if e != nil {
		return out, e
	}
	out = PriceListBatchInput{OperationID: add.OperationID, ExpectedRevision: add.ExpectedRevision, ProductIDs: add.ProductIDs, Create: add.Prices, Update: []PriceListPriceUpdate{}, Delete: []PriceListPriceRef{}}
	for _, key := range []string{"update", "delete"} {
		var rows []json.RawMessage
		if json.Unmarshal(f[key], &rows) != nil || rows == nil || len(rows) > 200 {
			return out, ErrPriceInput
		}
		names := []string{"id", "product_id", "variant_id"}
		if key == "update" {
			names = append(names, "amount", "min_quantity", "max_quantity")
		}
		for _, row := range rows {
			fields, e := listCreateObject(row, names)
			if e != nil {
				return out, e
			}
			for _, v := range fields {
				if len(v) == 0 || v[0] != '"' {
					return out, ErrPriceInput
				}
				var s string
				if json.Unmarshal(v, &s) != nil {
					return out, ErrPriceInput
				}
			}
			if key == "update" {
				var p PriceListPriceUpdate
				if json.Unmarshal(row, &p) != nil {
					return out, ErrPriceInput
				}
				out.Update = append(out.Update, p)
			} else {
				var p PriceListPriceRef
				if json.Unmarshal(row, &p) != nil {
					return out, ErrPriceInput
				}
				out.Delete = append(out.Delete, p)
			}
		}
	}
	if !validPriceListBatch(out) {
		return out, ErrPriceInput
	}
	return out, nil
}

// Retained rows allow a completed delete to be replayed, but only while current
// product/SKU/set/list membership is valid. A receipt never grants fresh access.
func batchPriceMembership(tx *gorm.DB, list string, p PriceListPriceRef) error {
	var found struct{ ID string }
	return tx.Table("pricing_prices p").Select("p.id").Joins("JOIN pricing_price_sets ps ON ps.id=p.price_set_id AND ps.deleted_at IS NULL").Joins("JOIN catalog_variant_price_sets vp ON vp.price_set_id=p.price_set_id AND vp.deleted_at IS NULL").Joins("JOIN catalog_variants v ON v.id=vp.variant_id AND v.deleted_at IS NULL").Joins("JOIN catalog_products cp ON cp.id=v.product_id AND cp.deleted_at IS NULL").Where("p.id=? AND p.price_list_id=? AND v.id=? AND cp.id=?", p.ID, list, p.VariantID, p.ProductID).Take(&found).Error
}

func (h Handler) BatchPriceListPrices(ctx context.Context, actor identity.User, id string, in PriceListBatchInput) (PriceListCreateResult, error) {
	var out PriceListCreateResult
	if !validProductID(id) || !validPriceListBatch(in) {
		return out, ErrPriceInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	request, _ := json.Marshal(struct {
		Action string `json:"action"`
		PriceListBatchInput
	}{"batch-prices", in})
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.ProductRead, authorization.PriceRead, authorization.PriceWrite); e != nil {
			return e
		}
		rt, e := lockPriceEdit(tx)
		if e != nil {
			return e
		}
		if !rt.Enabled {
			return ErrSchema
		}
		contextBefore, detailBefore, e := loadPriceListAdd(tx, id, in.ProductIDs, rt)
		if e != nil {
			return e
		}
		for _, p := range in.Update {
			if e := batchPriceMembership(tx, id, p.PriceListPriceRef); e != nil {
				return e
			}
		}
		for _, p := range in.Delete {
			if e := batchPriceMembership(tx, id, p); e != nil {
				return e
			}
		}
		var receipt struct{ ActorID, PriceListID, RequestJSON, ResultRevision string }
		e = tx.Table("catalog_price_list_edits").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.PriceListID != id || receipt.RequestJSON != string(request) {
				return ErrEditConflict
			}
			for _, p := range in.Create {
				var v struct{ ID string }
				if e := tx.Table("catalog_variants").Where("id=? AND product_id=? AND deleted_at IS NULL", p.VariantID, p.ProductID).Take(&v).Error; e != nil {
					return e
				}
			}
			out = PriceListCreateResult{id, receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if contextBefore.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		old := []json.RawMessage{}
		changed := map[string]bool{}
		// Read every existing participant before writing. The list/set conditions
		// above prevent a valid SKU from authorizing another list or base price.
		for _, p := range in.Update {
			c, e := loadPriceEdit(tx, p.ProductID, p.VariantID, p.ID, rt)
			if e != nil {
				return e
			}
			min, max := "", ""
			if c.EffectiveMinQuantity != nil {
				min = *c.EffectiveMinQuantity
			}
			if c.EffectiveMaxQuantity != nil {
				max = *c.EffectiveMaxQuantity
			}
			if min != p.MinQuantity || max != p.MaxQuantity {
				changed[p.ID] = true
			}
			audit, e := batchPriceAudit(tx, c)
			if e != nil {
				return e
			}
			old = append(old, audit)
		}
		for _, p := range in.Delete {
			c, e := loadPriceEdit(tx, p.ProductID, p.VariantID, p.ID, rt)
			if e != nil {
				return e
			}
			audit, e := batchPriceAudit(tx, c)
			if e != nil {
				return e
			}
			old = append(old, audit)
		}
		now := time.Now().UTC()
		for _, p := range in.Delete {
			q := tx.Exec("UPDATE pricing_prices SET deleted_at=?,updated_at=? WHERE id=? AND deleted_at IS NULL", now, now, p.ID)
			if q.Error != nil {
				return q.Error
			}
			if q.RowsAffected != 1 {
				return ErrEditConflict
			}
			if e := tx.Exec("UPDATE pricing_price_rules SET deleted_at=?,updated_at=? WHERE price_id=? AND deleted_at IS NULL", now, now, p.ID).Error; e != nil {
				return e
			}
		}
		for _, p := range in.Update {
			amount, _ := json.Marshal(map[string]any{"value": p.Amount, "precision": 20})
			min, rawMin := priceQuantityColumns(p.MinQuantity)
			max, rawMax := priceQuantityColumns(p.MaxQuantity)
			q := tx.Exec("UPDATE pricing_prices SET amount=?,raw_amount=?,min_quantity=?,raw_min_quantity=?,max_quantity=?,raw_max_quantity=?,updated_at=? WHERE id=? AND deleted_at IS NULL", p.Amount, string(amount), min, rawMin, max, rawMax, now, p.ID)
			if q.Error != nil {
				return q.Error
			}
			if q.RowsAffected != 1 {
				return ErrEditConflict
			}
		}
		created, e := insertListPrices(tx, id, in.Create, rt)
		if e != nil {
			return e
		}
		// Bounds may be exchanged between rows within a batch. Check the committed
		// shape after every update/delete, rather than rejecting an intermediate state.
		for _, p := range created {
			changed[p.ID] = true
		}
		if e := checkBatchPriceDuplicates(tx, id, changed); e != nil {
			return e
		}
		if e := tx.Exec("UPDATE catalog_price_edit_runtime SET revision=revision+1 WHERE id=1").Error; e != nil {
			return e
		}
		rt.Revision++
		after, e := loadPriceList(tx, id, rt)
		if e != nil {
			return e
		}
		out = PriceListCreateResult{id, after.Revision, false}
		b, _ := json.Marshal(struct {
			Context PriceListCreateContext `json:"context"`
			PriceListDetail
			Prices []json.RawMessage `json:"prices"`
		}{contextBefore, detailBefore, old})
		a, _ := json.Marshal(struct {
			PriceListDetail
			Created []savedListPrice       `json:"created"`
			Updated []PriceListPriceUpdate `json:"updated"`
			Deleted []PriceListPriceRef    `json:"deleted"`
		}{after, created, in.Update, in.Delete})
		return tx.Table("catalog_price_list_edits").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "price_list_id": id, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_revision": out.Revision, "created_at": now}).Error
	})
	return out, e
}

func checkBatchPriceDuplicates(tx *gorm.DB, list string, changed map[string]bool) error {
	if len(changed) == 0 {
		return nil
	}
	var prices []struct {
		ID, PriceSetID, CurrencyCode                             string
		MinQuantity, MaxQuantity, RawMinQuantity, RawMaxQuantity *string
	}
	if e := tx.Table("pricing_prices").Where("price_list_id=? AND deleted_at IS NULL", list).Order("id").Limit(10001).Scan(&prices).Error; e != nil {
		return e
	}
	if len(prices) > 10000 {
		return ErrSchema
	}
	var rules []struct {
		PriceID, Attribute, Operator, Value string
		Priority                            int
	}
	if e := tx.Table("pricing_price_rules r").Select("r.price_id,r.attribute,r.operator,r.value,r.priority").Joins("JOIN pricing_prices p ON p.id=r.price_id AND p.deleted_at IS NULL").Where("p.price_list_id=? AND r.deleted_at IS NULL", list).Order("r.price_id,r.attribute,r.operator,r.value,r.priority").Limit(10001).Scan(&rules).Error; e != nil {
		return e
	}
	if len(rules) > 10000 {
		return ErrSchema
	}
	byPrice := map[string][]string{}
	for _, r := range rules {
		rule, _ := json.Marshal([]any{r.Attribute, r.Operator, r.Value, r.Priority})
		byPrice[r.PriceID] = append(byPrice[r.PriceID], string(rule))
	}
	for _, rules := range byPrice {
		sort.Strings(rules)
	}
	seen := map[string]string{}
	for _, p := range prices {
		min, e := configuredQuantity(p.MinQuantity, p.RawMinQuantity)
		if e != nil {
			return e
		}
		max, e := configuredQuantity(p.MaxQuantity, p.RawMaxQuantity)
		if e != nil {
			return e
		}
		raw, _ := json.Marshal([]any{p.PriceSetID, p.CurrencyCode, min, max, byPrice[p.ID]})
		key := string(raw)
		if other, ok := seen[key]; ok && (changed[p.ID] || changed[other]) {
			return ErrEditConflict
		}
		seen[key] = p.ID
	}
	return nil
}

func batchPriceAudit(tx *gorm.DB, c PriceEditContext) (json.RawMessage, error) {
	var rules []struct {
		ID, Attribute, Operator, Value string
		Priority                       int
		CreatedAt, UpdatedAt           time.Time
	}
	if e := tx.Table("pricing_price_rules").Where("price_id=? AND deleted_at IS NULL", c.ID).Order("id").Limit(10001).Scan(&rules).Error; e != nil {
		return nil, e
	}
	if len(rules) > 10000 {
		return nil, ErrSchema
	}
	b, e := json.Marshal(struct {
		Price json.RawMessage `json:"price"`
		Rules any             `json:"rules"`
	}{json.RawMessage(quantityAudit(c)), rules})
	return b, e
}
