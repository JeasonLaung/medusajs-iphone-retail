package catalog

import (
	"context"
	"mime"
	"net/url"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PriceListBatchPrice struct {
	PriceListPriceUpdate
	PriceSetID   string           `json:"price_set_id"`
	CurrencyCode string           `json:"currency_code"`
	Rules        []AdminPriceRule `json:"rules"`
}
type PriceListBatchContext struct {
	PriceListCreateContext
	Prices []PriceListBatchPrice `json:"prices"`
}

func (h Handler) ReadPriceListBatch(ctx context.Context, actor identity.User, id string, ids []string) (PriceListBatchContext, error) {
	out := PriceListBatchContext{Prices: []PriceListBatchPrice{}}
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
		c, _, e := loadPriceListAdd(tx, id, ids, rt)
		if e != nil {
			return e
		}
		out.PriceListCreateContext = c
		seen := map[string]bool{}
		ruleCount := 0
		for _, p := range c.Products {
			for _, v := range p.Variants {
				if v.PriceSetID == nil || seen[*v.PriceSetID] {
					continue
				}
				seen[*v.PriceSetID] = true
				var rows []struct{ ID string }
				if e := tx.Table("pricing_prices").Select("id").Where("price_list_id=? AND price_set_id=? AND deleted_at IS NULL", id, *v.PriceSetID).Order("id").Limit(501).Scan(&rows).Error; e != nil {
					return e
				}
				if len(out.Prices)+len(rows) > 500 {
					return ErrSchema
				}
				for _, row := range rows {
					price, e := loadPriceEdit(tx, p.ID, v.ID, row.ID, rt)
					if e != nil {
						return e
					}
					min, max := "", ""
					if price.EffectiveMinQuantity != nil {
						min = *price.EffectiveMinQuantity
					}
					if price.EffectiveMaxQuantity != nil {
						max = *price.EffectiveMaxQuantity
					}
					rules := []AdminPriceRule{}
					if e := tx.Table("pricing_price_rules").Where("price_id=? AND deleted_at IS NULL", row.ID).Order("id").Limit(10001).Scan(&rules).Error; e != nil {
						return e
					}
					ruleCount += len(rules)
					if ruleCount > 10000 {
						return ErrSchema
					}
					out.Prices = append(out.Prices, PriceListBatchPrice{PriceListPriceUpdate{PriceListPriceRef{row.ID, p.ID, v.ID}, price.EffectiveAmount, min, max}, price.PriceSetID, price.CurrencyCode, rules})
				}
			}
		}
		return nil
	})
	return out, e
}
func (h Handler) PriceListBatchHTTP(c *gin.Context) {
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
		out, e := h.ReadPriceListBatch(ctx, actor, id, ids)
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
	in, e := decodePriceListBatch(c.Request.Body)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	out, e := h.BatchPriceListPrices(ctx, actor, id, in)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	c.JSON(200, out)
}
