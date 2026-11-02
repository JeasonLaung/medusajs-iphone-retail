package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PriceRemoveInput struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
}

func validPriceRemove(in PriceRemoveInput) bool {
	return validPriceAmount(PriceAmountInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, Amount: "0.000000000000000000"})
}
func decodePriceRemove(r io.Reader) (PriceRemoveInput, error) {
	var in PriceRemoveInput
	raw, e := io.ReadAll(io.LimitReader(r, 2049))
	if e != nil || len(raw) > 2048 || !utf8.Valid(raw) {
		return in, ErrPriceInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return in, ErrPriceInput
	}
	fields := map[string]string{}
	for d.More() {
		token, e := d.Token()
		name, ok := token.(string)
		if e != nil || !ok || (name != "operation_id" && name != "expected_revision") {
			return in, ErrPriceInput
		}
		if _, exists := fields[name]; exists {
			return in, ErrPriceInput
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil || len(raw) == 0 || raw[0] != '"' {
			return in, ErrPriceInput
		}
		var v string
		if json.Unmarshal(raw, &v) != nil {
			return in, ErrPriceInput
		}
		fields[name] = v
	}
	if token, e = d.Token(); e != nil || token != json.Delim('}') || len(fields) != 2 || d.Decode(new(any)) != io.EOF {
		return in, ErrPriceInput
	}
	in = PriceRemoveInput{fields["operation_id"], fields["expected_revision"]}
	if !validPriceRemove(in) {
		return in, ErrPriceInput
	}
	return in, nil
}

func (h Handler) RemovePrice(ctx context.Context, actor identity.User, productID, variantID, priceID string, in PriceRemoveInput) (EditResult, error) {
	var out EditResult
	if !validPricePath(productID, variantID, priceID) || !validPriceRemove(in) {
		return out, ErrPriceInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	// Bind the command kind in the shared receipt namespace. An amount/create
	// receipt can never authorize a removal with the same operation UUID.
	request, _ := json.Marshal(struct {
		Action string `json:"action"`
		PriceRemoveInput
	}{"remove", in})
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
		// A completed removal has no active price. Check current product/SKU/set
		// membership against the retained row before consulting its receipt.
		var membership struct {
			ID        string
			DeletedAt *time.Time
		}
		e = tx.Table("pricing_prices p").Select("p.id,p.deleted_at").Joins("JOIN pricing_price_sets ps ON ps.id=p.price_set_id AND ps.deleted_at IS NULL").Joins("JOIN catalog_variant_price_sets vp ON vp.price_set_id=p.price_set_id AND vp.deleted_at IS NULL").Joins("JOIN catalog_variants v ON v.id=vp.variant_id AND v.deleted_at IS NULL").Joins("JOIN catalog_products cp ON cp.id=v.product_id AND cp.deleted_at IS NULL").Where("p.id=? AND v.id=? AND cp.id=?", priceID, variantID, productID).Take(&membership).Error
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, ProductID, VariantID, PriceID, RequestJSON, ResultRevision string }
		e = tx.Table("catalog_price_edits").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.ProductID != productID || receipt.VariantID != variantID || receipt.PriceID != priceID || receipt.RequestJSON != string(request) {
				return ErrEditConflict
			}
			if membership.DeletedAt == nil {
				return ErrEditConflict
			}
			out = EditResult{receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		before, e := loadPriceEdit(tx, productID, variantID, priceID, rt)
		if e != nil {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		var rules []struct {
			ID, Attribute, Operator, Value string
			Priority                       int
			CreatedAt, UpdatedAt           time.Time
		}
		if e = tx.Table("pricing_price_rules").Where("price_id=? AND deleted_at IS NULL", priceID).Order("id").Scan(&rules).Error; e != nil {
			return e
		}
		now := time.Now().UTC()
		result := tx.Exec("UPDATE pricing_prices SET deleted_at=?,updated_at=? WHERE id=? AND deleted_at IS NULL", now, now, priceID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrEditConflict
		}
		if e = tx.Exec("UPDATE pricing_price_rules SET deleted_at=?,updated_at=? WHERE price_id=? AND deleted_at IS NULL", now, now, priceID).Error; e != nil {
			return e
		}
		if e = tx.Exec("UPDATE catalog_price_edit_runtime SET revision=revision+1 WHERE id=1").Error; e != nil {
			return e
		}
		rt.Revision++
		after := struct {
			ID                    string    `json:"id"`
			DeletedAt             time.Time `json:"deleted_at"`
			ConfigurationRevision uint64    `json:"configuration_revision"`
		}{priceID, now, rt.Revision}
		a, _ := json.Marshal(after)
		sum := sha256.Sum256(a)
		out.Revision = hex.EncodeToString(sum[:])
		b, _ := json.Marshal(map[string]any{"price": before, "raw_amount": before.rawAmount, "rules": rules})
		return tx.Table("catalog_price_edits").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "product_id": productID, "variant_id": variantID, "price_id": priceID, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_revision": out.Revision, "created_at": now}).Error
	})
	return out, e
}
func (h Handler) PriceRemoveHTTP(c *gin.Context) {
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
	in, e := decodePriceRemove(c.Request.Body)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	out, e := h.RemovePrice(ctx, actor, c.Param("id"), c.Param("variant_id"), c.Param("price_id"), in)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	c.JSON(200, out)
}
