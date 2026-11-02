package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"mime"
	"time"
	"unicode/utf8"
)

type PriceQuantityInput struct {
	OperationID      string  `json:"operation_id"`
	ExpectedRevision string  `json:"expected_revision"`
	MinQuantity      *string `json:"min_quantity"`
	MaxQuantity      *string `json:"max_quantity"`
}

func validPriceQuantityEdit(in PriceQuantityInput) bool {
	if !validPriceRemove(PriceRemoveInput{in.OperationID, in.ExpectedRevision}) {
		return false
	}
	min, max := "", ""
	if in.MinQuantity != nil {
		min = *in.MinQuantity
		if min == "" {
			return false
		}
	}
	if in.MaxQuantity != nil {
		max = *in.MaxQuantity
		if max == "" {
			return false
		}
	}
	return validPriceQuantities(min, max)
}
func decodePriceQuantityEdit(r io.Reader) (PriceQuantityInput, error) {
	var in PriceQuantityInput
	raw, e := io.ReadAll(io.LimitReader(r, 2049))
	if e != nil || len(raw) > 2048 || !utf8.Valid(raw) {
		return in, ErrPriceInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return in, ErrPriceInput
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, e := d.Token()
		key, ok := token.(string)
		if e != nil || !ok || (key != "operation_id" && key != "expected_revision" && key != "min_quantity" && key != "max_quantity") {
			return in, ErrPriceInput
		}
		if _, ok := fields[key]; ok {
			return in, ErrPriceInput
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return in, ErrPriceInput
		}
		fields[key] = value
	}
	if token, e = d.Token(); e != nil || token != json.Delim('}') || len(fields) != 4 || d.Decode(new(any)) != io.EOF {
		return in, ErrPriceInput
	}
	for _, key := range []string{"operation_id", "expected_revision"} {
		if len(fields[key]) == 0 || fields[key][0] != '"' {
			return in, ErrPriceInput
		}
	}
	for _, key := range []string{"min_quantity", "max_quantity"} {
		if !bytes.Equal(fields[key], []byte("null")) && (len(fields[key]) == 0 || fields[key][0] != '"') {
			return in, ErrPriceInput
		}
	}
	b, _ := json.Marshal(fields)
	if json.Unmarshal(b, &in) != nil || !validPriceQuantityEdit(in) {
		return in, ErrPriceInput
	}
	return in, nil
}
func quantityAudit(in PriceEditContext) string {
	b, _ := json.Marshal(map[string]any{"price": in, "raw_amount": in.rawAmount, "raw_min_quantity": in.rawMinQuantity, "raw_max_quantity": in.rawMaxQuantity})
	return string(b)
}
func (h Handler) EditPriceQuantity(ctx context.Context, actor identity.User, p, v, id string, in PriceQuantityInput) (EditResult, error) {
	var out EditResult
	if !validPricePath(p, v, id) || !validPriceQuantityEdit(in) {
		return out, ErrPriceInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	request, _ := json.Marshal(struct {
		Action string `json:"action"`
		PriceQuantityInput
	}{"quantity", in})
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
		before, e := loadPriceEdit(tx, p, v, id, rt)
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, ProductID, VariantID, PriceID, RequestJSON, ResultRevision string }
		e = tx.Table("catalog_price_edits").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.ProductID != p || receipt.VariantID != v || receipt.PriceID != id || receipt.RequestJSON != string(request) {
				return ErrEditConflict
			}
			out = EditResult{receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		var min, rawMin, max, rawMax any
		if in.MinQuantity != nil {
			min, rawMin = priceQuantityColumns(*in.MinQuantity)
		}
		if in.MaxQuantity != nil {
			max, rawMax = priceQuantityColumns(*in.MaxQuantity)
		}
		update := tx.Exec("UPDATE pricing_prices SET min_quantity=?,raw_min_quantity=?,max_quantity=?,raw_max_quantity=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND deleted_at IS NULL", min, rawMin, max, rawMax, id)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrEditConflict
		}
		if e = tx.Exec("UPDATE catalog_price_edit_runtime SET revision=revision+1 WHERE id=1").Error; e != nil {
			return e
		}
		rt.Revision++
		after, e := loadPriceEdit(tx, p, v, id, rt)
		if e != nil {
			return e
		}
		out.Revision = after.Revision
		return tx.Table("catalog_price_edits").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "product_id": p, "variant_id": v, "price_id": id, "request_json": string(request), "before_json": quantityAudit(before), "after_json": quantityAudit(after), "result_revision": out.Revision, "created_at": time.Now().UTC()}).Error
	})
	return out, e
}
func (h Handler) PriceQuantityHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		priceEditFail(c, ErrPriceInput)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "catalog.content_type", "JSON required")
		return
	}
	in, e := decodePriceQuantityEdit(c.Request.Body)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	out, e := h.EditPriceQuantity(ctx, actor, c.Param("id"), c.Param("variant_id"), c.Param("price_id"), in)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	c.JSON(200, out)
}
