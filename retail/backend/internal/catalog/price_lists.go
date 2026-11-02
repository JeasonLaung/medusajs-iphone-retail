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
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PriceListInput struct {
	OperationID      string  `json:"operation_id"`
	ExpectedRevision string  `json:"expected_revision"`
	Title            string  `json:"title"`
	Description      string  `json:"description"`
	Status           string  `json:"status"`
	Type             string  `json:"type"`
	StartsAt         *string `json:"starts_at"`
	EndsAt           *string `json:"ends_at"`
}
type PriceListDetail struct {
	AdminPriceList
	Description string `json:"description"`
	PriceCount  int64  `json:"price_count"`
	Enabled     bool   `json:"enabled"`
	Revision    string `json:"revision"`
}

func priceListTime(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	t, e := time.Parse(time.RFC3339Nano, *value)
	if e != nil || t.Year() < 1000 || t.Year() > 9999 || t.Nanosecond()%1000 != 0 {
		return nil, ErrPriceInput
	}
	t = t.UTC()
	return &t, nil
}
func validPriceListInput(in PriceListInput) bool {
	if !validPriceRemove(PriceRemoveInput{in.OperationID, in.ExpectedRevision}) || !utf8.ValidString(in.Title) || !utf8.ValidString(in.Description) || strings.TrimSpace(in.Title) == "" || len(in.Title) > 4096 || strings.TrimSpace(in.Description) == "" || len(in.Description) > 65536 || (in.Status != "active" && in.Status != "draft") || (in.Type != "sale" && in.Type != "override") {
		return false
	}
	_, a := priceListTime(in.StartsAt)
	_, b := priceListTime(in.EndsAt)
	return a == nil && b == nil
}
func decodePriceListInput(r io.Reader) (PriceListInput, error) {
	var in PriceListInput
	raw, e := io.ReadAll(io.LimitReader(r, 73729))
	if e != nil || len(raw) > 73728 || !utf8.Valid(raw) {
		return in, ErrPriceInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return in, ErrPriceInput
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		tok, e := d.Token()
		key, ok := tok.(string)
		if e != nil || !ok {
			return in, ErrPriceInput
		}
		switch key {
		case "operation_id", "expected_revision", "title", "description", "status", "type", "starts_at", "ends_at":
		default:
			return in, ErrPriceInput
		}
		if _, exists := fields[key]; exists {
			return in, ErrPriceInput
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return in, ErrPriceInput
		}
		if len(value) == 0 || (value[0] != '"' && ((key != "starts_at" && key != "ends_at") || !bytes.Equal(value, []byte("null")))) {
			return in, ErrPriceInput
		}
		fields[key] = value
	}
	if tok, e = d.Token(); e != nil || tok != json.Delim('}') || len(fields) != 8 || d.Decode(new(any)) != io.EOF {
		return in, ErrPriceInput
	}
	b, _ := json.Marshal(fields)
	if json.Unmarshal(b, &in) != nil || !validPriceListInput(in) {
		return in, ErrPriceInput
	}
	return in, nil
}
func loadPriceList(tx *gorm.DB, id string, rt priceEditRuntime) (PriceListDetail, error) {
	out := PriceListDetail{Enabled: rt.Enabled}
	var row struct {
		ID, Title, Description, Status, Type string
		StartsAt, EndsAt                     *time.Time
		RulesCount                           *int
		CreatedAt, UpdatedAt                 time.Time
		Metadata                             *string
	}
	if e := tx.Table("pricing_price_lists").Where("id=? AND deleted_at IS NULL", id).Take(&row).Error; e != nil {
		return out, e
	}
	var rules []struct {
		ID, Attribute string
		Value         *string
		UpdatedAt     time.Time
	}
	if e := tx.Table("pricing_price_list_rules").Where("price_list_id=? AND deleted_at IS NULL", id).Order("id").Limit(10001).Scan(&rules).Error; e != nil || len(rules) > 10000 {
		return out, ErrSchema
	}
	var prices []struct {
		ID, PriceSetID string
		UpdatedAt      time.Time
	}
	if e := tx.Table("pricing_prices").Where("price_list_id=? AND deleted_at IS NULL", id).Order("id").Limit(10001).Scan(&prices).Error; e != nil || len(prices) > 10000 {
		return out, ErrSchema
	}
	out.AdminPriceList = AdminPriceList{ID: row.ID, Title: row.Title, Status: row.Status, Type: row.Type, StartsAt: row.StartsAt, EndsAt: row.EndsAt, RulesCount: row.RulesCount, Rules: []AdminPriceListRule{}}
	for _, r := range rules {
		out.Rules = append(out.Rules, AdminPriceListRule{r.ID, r.Attribute, r.Value})
	}
	out.Description = row.Description
	out.PriceCount = int64(len(prices))
	b, _ := json.Marshal([]any{row, rules, prices, rt.Revision})
	sum := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(sum[:])
	return out, nil
}
func (h Handler) ReadPriceList(ctx context.Context, actor identity.User, id string) (PriceListDetail, error) {
	var out PriceListDetail
	if !validProductID(id) {
		return out, ErrPriceInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.PriceRead); e != nil {
			return e
		}
		rt, e := lockPriceEdit(tx)
		if e != nil {
			return e
		}
		out, e = loadPriceList(tx, id, rt)
		return e
	})
	return out, e
}
func (h Handler) EditPriceList(ctx context.Context, actor identity.User, id string, in PriceListInput) (EditResult, error) {
	var out EditResult
	if !validProductID(id) || !validPriceListInput(in) {
		return out, ErrPriceInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	request, _ := json.Marshal(in)
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
		before, e := loadPriceList(tx, id, rt)
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, PriceListID, RequestJSON, ResultRevision string }
		e = tx.Table("catalog_price_list_edits").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.PriceListID != id || receipt.RequestJSON != string(request) {
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
		starts, _ := priceListTime(in.StartsAt)
		ends, _ := priceListTime(in.EndsAt)
		update := tx.Exec("UPDATE pricing_price_lists SET title=?,description=?,status=?,type=?,starts_at=?,ends_at=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND deleted_at IS NULL", in.Title, in.Description, in.Status, in.Type, starts, ends, id)
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
		after, e := loadPriceList(tx, id, rt)
		if e != nil {
			return e
		}
		out.Revision = after.Revision
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(after)
		return tx.Table("catalog_price_list_edits").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "price_list_id": id, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_revision": out.Revision, "created_at": time.Now().UTC()}).Error
	})
	return out, e
}
func (h Handler) PriceListsHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	id := c.Param("list_id")
	if id == "" {
		values, e := url.ParseQuery(c.Request.URL.RawQuery)
		q, qe := parsePricePage(values)
		if e != nil || qe != nil {
			priceEditFail(c, ErrPriceInput)
			return
		}
		if h.DB == nil {
			priceEditFail(c, ErrSchema)
			return
		}
		lists := []AdminPriceList{}
		var count int64
		e = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := authorization.CheckTx(ctx, tx, actor, authorization.PriceRead); e != nil {
				return e
			}
			if _, e := lockPriceEdit(tx); e != nil {
				return e
			}
			base := tx.Table("pricing_price_lists").Where("deleted_at IS NULL")
			if e := base.Count(&count).Error; e != nil {
				return e
			}
			return base.Select("id,title,status,type,starts_at,ends_at,rules_count").Order("created_at DESC,id").Limit(q.Limit).Offset(q.Offset).Scan(&lists).Error
		})
		if e != nil {
			priceEditFail(c, e)
			return
		}
		c.JSON(200, gin.H{"price_lists": lists, "count": count, "limit": q.Limit, "offset": q.Offset})
		return
	}
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		priceEditFail(c, ErrPriceInput)
		return
	}
	if c.Request.Method == "GET" {
		out, e := h.ReadPriceList(ctx, actor, id)
		if e != nil {
			priceEditFail(c, e)
			return
		}
		c.JSON(200, out)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "catalog.content_type", "JSON required")
		return
	}
	in, e := decodePriceListInput(c.Request.Body)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	out, e := h.EditPriceList(ctx, actor, id, in)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	c.JSON(200, out)
}
