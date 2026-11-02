package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Retain the source cascade's affected identities and exact monetary columns.
// The project preserves soft-deleted rows instead of physically removing them.
func (h Handler) RemovePriceList(ctx context.Context, actor identity.User, id string, in PriceRemoveInput) (EditResult, error) {
	var out EditResult
	if !validProductID(id) || !validPriceRemove(in) {
		return out, ErrPriceInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	request, _ := json.Marshal(struct {
		Action string `json:"action"`
		PriceRemoveInput
	}{"remove", in})
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.PriceRead, authorization.PriceWrite); e != nil {
			return e
		}
		rt, e := lockPriceEdit(tx)
		if e != nil {
			return e
		}
		if !rt.Enabled {
			return ErrSchema
		}
		var retained struct {
			ID        string
			DeletedAt *time.Time
		}
		if e = tx.Table("pricing_price_lists").Select("id,deleted_at").Where("id=?", id).Take(&retained).Error; e != nil {
			return e
		}
		var receipt struct{ ActorID, PriceListID, RequestJSON, ResultRevision string }
		e = tx.Table("catalog_price_list_edits").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.PriceListID != id || receipt.RequestJSON != string(request) || retained.DeletedAt == nil {
				return ErrEditConflict
			}
			out = EditResult{receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		before, e := loadPriceList(tx, id, rt)
		if e != nil {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		var prices []struct {
			ID, PriceSetID, CurrencyCode, Amount, RawAmount          string
			MinQuantity, MaxQuantity, RawMinQuantity, RawMaxQuantity *string
			RulesCount                                               *int
			CreatedAt, UpdatedAt                                     time.Time
			DeletedAt                                                *time.Time
		}
		if e = tx.Table("pricing_prices").Where("price_list_id=?", id).Order("id").Limit(10001).Scan(&prices).Error; e != nil {
			return e
		}
		if len(prices) > 10000 {
			return ErrSchema
		}
		var rules []struct {
			ID, PriceID, Attribute, Operator, Value string
			Priority                                int
			CreatedAt, UpdatedAt                    time.Time
			DeletedAt                               *time.Time
		}
		if e = tx.Table("pricing_price_rules r").Select("r.*").Joins("JOIN pricing_prices p ON p.id=r.price_id").Where("p.price_list_id=?", id).Order("r.id").Limit(10001).Scan(&rules).Error; e != nil {
			return e
		}
		if len(rules) > 10000 {
			return ErrSchema
		}
		now := time.Now().UTC()
		result := tx.Exec("UPDATE pricing_price_lists SET deleted_at=?,updated_at=? WHERE id=? AND deleted_at IS NULL", now, now, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrEditConflict
		}
		if e = tx.Exec("UPDATE pricing_price_list_rules SET deleted_at=?,updated_at=? WHERE price_list_id=? AND deleted_at IS NULL", now, now, id).Error; e != nil {
			return e
		}
		if e = tx.Exec("UPDATE pricing_price_rules r JOIN pricing_prices p ON p.id=r.price_id SET r.deleted_at=?,r.updated_at=? WHERE p.price_list_id=? AND r.deleted_at IS NULL", now, now, id).Error; e != nil {
			return e
		}
		if e = tx.Exec("UPDATE pricing_prices SET deleted_at=?,updated_at=? WHERE price_list_id=? AND deleted_at IS NULL", now, now, id).Error; e != nil {
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
		}{id, now, rt.Revision}
		a, _ := json.Marshal(after)
		sum := sha256.Sum256(a)
		out.Revision = hex.EncodeToString(sum[:])
		b, _ := json.Marshal(map[string]any{"price_list": before, "prices": prices, "price_rules": rules})
		return tx.Table("catalog_price_list_edits").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "price_list_id": id, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_revision": out.Revision, "created_at": now}).Error
	})
	return out, e
}
func (h Handler) PriceListRemoveHTTP(c *gin.Context) {
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
	out, e := h.RemovePriceList(ctx, actor, c.Param("list_id"), in)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	c.JSON(200, out)
}
