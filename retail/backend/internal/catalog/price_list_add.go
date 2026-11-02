package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"net/url"
	"time"
	"unicode/utf8"
)

type PriceListAddInput struct {
	OperationID      string             `json:"operation_id"`
	ExpectedRevision string             `json:"expected_revision"`
	ProductIDs       []string           `json:"product_ids"`
	Prices           []InitialListPrice `json:"prices"`
}

func validPriceListAdd(in PriceListAddInput) bool {
	return validPriceListCreate(PriceListCreateInput{PriceListInput: PriceListInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, Title: "validation", Description: "validation", Status: "active", Type: "sale"}, ProductIDs: in.ProductIDs, CustomerGroupIDs: []string{}, Prices: in.Prices})
}
func decodePriceListAdd(r io.Reader) (PriceListAddInput, error) {
	var out PriceListAddInput
	raw, e := io.ReadAll(io.LimitReader(r, 262145))
	if e != nil || len(raw) > 262144 || !utf8.Valid(raw) {
		return out, ErrPriceInput
	}
	f, e := listCreateObject(raw, []string{"operation_id", "expected_revision", "product_ids", "prices"})
	if e != nil {
		return out, e
	}
	for k, v := range map[string]string{"title": "validation", "description": "validation", "status": "active", "type": "sale"} {
		f[k], _ = json.Marshal(v)
	}
	f["starts_at"] = json.RawMessage("null")
	f["ends_at"] = json.RawMessage("null")
	f["customer_group_ids"] = json.RawMessage("[]")
	b, _ := json.Marshal(f)
	in, e := decodePriceListCreate(bytes.NewReader(b))
	if e != nil {
		return out, e
	}
	return PriceListAddInput{in.OperationID, in.ExpectedRevision, in.ProductIDs, in.Prices}, nil
}
func loadPriceListAdd(tx *gorm.DB, id string, ids []string, rt priceEditRuntime) (PriceListCreateContext, PriceListDetail, error) {
	detail, e := loadPriceList(tx, id, rt)
	if e != nil {
		return PriceListCreateContext{}, detail, e
	}
	c, e := loadPriceListCreate(tx, ids, rt)
	if e != nil {
		return c, detail, e
	}
	raw, _ := json.Marshal([]string{id, detail.Revision, c.Revision})
	sum := sha256.Sum256(raw)
	c.Revision = hex.EncodeToString(sum[:])
	return c, detail, nil
}
func (h Handler) ReadPriceListAdd(ctx context.Context, actor identity.User, id string, ids []string) (PriceListCreateContext, error) {
	var out PriceListCreateContext
	if !validProductID(id) || !validCreateListIDs(ids, 0, 20) {
		return out, ErrPriceInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.ProductRead, authorization.PriceRead); e != nil {
			return e
		}
		rt, e := lockPriceEdit(tx)
		if e != nil {
			return e
		}
		out, _, e = loadPriceListAdd(tx, id, ids, rt)
		return e
	})
	return out, e
}
func (h Handler) AddPriceListPrices(ctx context.Context, actor identity.User, id string, in PriceListAddInput) (PriceListCreateResult, error) {
	var out PriceListCreateResult
	if !validProductID(id) || !validPriceListAdd(in) {
		return out, ErrPriceInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	request, _ := json.Marshal(struct {
		Action string `json:"action"`
		PriceListAddInput
	}{"add-prices", in})
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
		before, detail, e := loadPriceListAdd(tx, id, in.ProductIDs, rt)
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, PriceListID, RequestJSON, ResultRevision string }
		e = tx.Table("catalog_price_list_edits").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.PriceListID != id || receipt.RequestJSON != string(request) {
				return ErrEditConflict
			}
			for _, p := range in.Prices {
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
		if in.ExpectedRevision != before.Revision {
			return ErrEditConflict
		}
		saved, e := insertListPrices(tx, id, in.Prices, rt)
		if e != nil {
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
		}{before, detail})
		a, _ := json.Marshal(struct {
			PriceListDetail
			Prices []savedListPrice `json:"prices"`
		}{after, saved})
		return tx.Table("catalog_price_list_edits").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "price_list_id": id, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_revision": out.Revision, "created_at": time.Now().UTC()}).Error
	})
	return out, e
}
func (h Handler) PriceListAddHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	id := c.Param("list_id")
	if c.Request.Method == "GET" {
		q, e := url.ParseQuery(c.Request.URL.RawQuery)
		if e != nil || c.Request.URL.ForceQuery && c.Request.URL.RawQuery == "" || len(q) > 1 || len(q) == 1 && q["product_id"] == nil {
			priceEditFail(c, ErrPriceInput)
			return
		}
		ids := q["product_id"]
		if ids == nil {
			ids = []string{}
		}
		out, e := h.ReadPriceListAdd(ctx, actor, id, ids)
		if e != nil {
			priceEditFail(c, e)
			return
		}
		c.JSON(200, out)
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
	in, e := decodePriceListAdd(c.Request.Body)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	out, e := h.AddPriceListPrices(ctx, actor, id, in)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	c.JSON(200, out)
}
