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
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"mime"
	"regexp"
	"sort"
	"time"
	"unicode/utf8"
)

type PriceCreateInput struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	CurrencyCode     string `json:"currency_code"`
	Amount           string `json:"amount"`
}
type PriceCreateContext struct {
	ProductID              string   `json:"product_id"`
	VariantID              string   `json:"variant_id"`
	PriceSetID             *string  `json:"price_set_id"`
	Currencies             []string `json:"currencies"`
	ExistingBaseCurrencies []string `json:"existing_base_currencies"`
	SharedVariants         int      `json:"shared_variants"`
	Enabled                bool     `json:"enabled"`
	Revision               string   `json:"revision"`
}
type PriceCreateResult struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}

var createCurrencyPattern = regexp.MustCompile(`^[a-z]{3}$`)

func validPriceCreate(in PriceCreateInput) bool {
	return validPriceAmount(PriceAmountInput{in.OperationID, in.ExpectedRevision, in.Amount}) && createCurrencyPattern.MatchString(in.CurrencyCode)
}
func decodePriceCreate(r io.Reader) (PriceCreateInput, error) {
	var in PriceCreateInput
	b, e := io.ReadAll(io.LimitReader(r, 2049))
	if e != nil || len(b) > 2048 || !utf8.Valid(b) {
		return in, ErrPriceInput
	}
	d := json.NewDecoder(bytes.NewReader(b))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return in, ErrPriceInput
	}
	fields := map[string]string{}
	for d.More() {
		tok, e := d.Token()
		k, ok := tok.(string)
		if e != nil || !ok {
			return in, ErrPriceInput
		}
		if _, exists := fields[k]; exists {
			return in, ErrPriceInput
		}
		if k != "operation_id" && k != "expected_revision" && k != "currency_code" && k != "amount" {
			return in, ErrPriceInput
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil || len(raw) == 0 || raw[0] != '"' {
			return in, ErrPriceInput
		}
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return in, ErrPriceInput
		}
		fields[k] = s
	}
	if tok, e = d.Token(); e != nil || tok != json.Delim('}') || len(fields) != 4 || d.Decode(new(any)) != io.EOF {
		return in, ErrPriceInput
	}
	in = PriceCreateInput{fields["operation_id"], fields["expected_revision"], fields["currency_code"], fields["amount"]}
	if !validPriceCreate(in) {
		return in, ErrPriceInput
	}
	return in, nil
}
func loadPriceCreate(tx *gorm.DB, p, v string, rt priceEditRuntime) (PriceCreateContext, error) {
	out := PriceCreateContext{ProductID: p, VariantID: v, Enabled: rt.Enabled, Currencies: []string{}, ExistingBaseCurrencies: []string{}}
	var variant struct {
		ID        string
		UpdatedAt time.Time
	}
	if e := tx.Table("catalog_variants v").Select("v.id,v.updated_at").Joins("JOIN catalog_products p ON p.id=v.product_id AND p.deleted_at IS NULL").Where("v.id=? AND p.id=? AND v.deleted_at IS NULL", v, p).Take(&variant).Error; e != nil {
		return out, e
	}
	var links []struct{ ID, PriceSetID string }
	if tx.Table("catalog_variant_price_sets vp").Select("vp.id,vp.price_set_id").Joins("JOIN pricing_price_sets ps ON ps.id=vp.price_set_id AND ps.deleted_at IS NULL").Where("vp.variant_id=? AND vp.deleted_at IS NULL", v).Order("vp.price_set_id").Limit(2).Scan(&links).Error != nil {
		return out, ErrSchema
	}
	if len(links) > 1 {
		return out, ErrEditConflict
	}
	var prices []struct {
		ID, CurrencyCode, Amount, RawAmount   string
		MinQuantity, MaxQuantity, PriceListID *string
		RulesCount                            *int
		UpdatedAt                             time.Time
	}
	var rules []struct {
		ID, PriceID, Attribute, Operator, Value string
		Priority                                int
		UpdatedAt                               time.Time
	}
	var members []struct{ ID, VariantID, ProductID string }
	if len(links) == 1 {
		set := links[0].PriceSetID
		out.PriceSetID = &set
		if tx.Table("pricing_prices").Where("price_set_id=? AND deleted_at IS NULL", set).Order("id").Limit(10001).Scan(&prices).Error != nil || len(prices) > 10000 {
			return out, ErrSchema
		}
		if tx.Table("pricing_price_rules r").Select("r.id,r.price_id,r.attribute,r.operator,r.value,r.priority,r.updated_at").Joins("JOIN pricing_prices p ON p.id=r.price_id AND p.deleted_at IS NULL").Where("p.price_set_id=? AND r.deleted_at IS NULL", set).Order("r.id").Limit(10001).Scan(&rules).Error != nil || len(rules) > 10000 {
			return out, ErrSchema
		}
		ruled := map[string]bool{}
		for _, r := range rules {
			ruled[r.PriceID] = true
		}
		existing := map[string]bool{}
		for _, price := range prices {
			if price.PriceListID == nil && price.MinQuantity == nil && price.MaxQuantity == nil && !ruled[price.ID] {
				existing[price.CurrencyCode] = true
			}
		}
		for currency := range existing {
			out.ExistingBaseCurrencies = append(out.ExistingBaseCurrencies, currency)
		}
		if tx.Table("catalog_variant_price_sets vp").Select("vp.id,vp.variant_id,v.product_id").Joins("JOIN catalog_variants v ON v.id=vp.variant_id AND v.deleted_at IS NULL").Joins("JOIN catalog_products p ON p.id=v.product_id AND p.deleted_at IS NULL").Where("vp.price_set_id=? AND vp.deleted_at IS NULL", set).Order("vp.id").Limit(10001).Scan(&members).Error != nil || len(members) > 10000 {
			return out, ErrSchema
		}
		seen := map[string]bool{}
		for _, m := range members {
			seen[m.VariantID] = true
		}
		out.SharedVariants = len(seen)
	}
	if tx.Table("commerce_currencies").Where("deleted_at IS NULL AND code REGEXP '^[a-z]{3}$'").Order("code").Limit(1001).Pluck("code", &out.Currencies).Error != nil || len(out.Currencies) > 1000 {
		return out, ErrSchema
	}
	sort.Strings(out.ExistingBaseCurrencies)
	b, _ := json.Marshal([]any{p, v, variant, links, prices, rules, members, out.Currencies, rt.Revision})
	sum := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(sum[:])
	return out, nil
}
func (h Handler) ReadPriceCreate(ctx context.Context, actor identity.User, p, v string) (PriceCreateContext, error) {
	var out PriceCreateContext
	if !validProductID(p) || !validProductID(v) {
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
		out, e = loadPriceCreate(tx, p, v, rt)
		return e
	})
	return out, e
}
func (h Handler) CreateBasePrice(ctx context.Context, actor identity.User, p, v string, in PriceCreateInput) (PriceCreateResult, error) {
	var out PriceCreateResult
	if !validProductID(p) || !validProductID(v) || !validPriceCreate(in) {
		return out, ErrPriceInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	request, _ := json.Marshal(in)
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
		before, e := loadPriceCreate(tx, p, v, rt)
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, ProductID, VariantID, PriceID, RequestJSON, ResultRevision string }
		e = tx.Table("catalog_price_edits").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.ProductID != p || receipt.VariantID != v || receipt.RequestJSON != string(request) {
				return ErrEditConflict
			}
			// Replay must still resolve the created price through this current SKU.
			if _, e := loadPriceEdit(tx, p, v, receipt.PriceID, rt); e != nil {
				return e
			}
			out = PriceCreateResult{receipt.PriceID, receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		found := false
		for _, c := range before.Currencies {
			if c == in.CurrencyCode {
				found = true
			}
		}
		if !found {
			return ErrPriceInput
		}
		for _, c := range before.ExistingBaseCurrencies {
			if c == in.CurrencyCode {
				return ErrEditConflict
			}
		}
		set := ""
		if before.PriceSetID != nil {
			set = *before.PriceSetID
		} else {
			set = "pset_" + uuid.NewString()
			if tx.Exec("INSERT INTO pricing_price_sets(id,created_at,updated_at) VALUES(?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", set).Error != nil {
				return ErrSchema
			}
			// A soft-deleted membership is not revived; use a fresh price set and link.
			if tx.Exec("INSERT INTO catalog_variant_price_sets(id,variant_id,price_set_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "pvps_"+uuid.NewString(), v, set).Error != nil {
				return ErrSchema
			}
		}
		id := "price_" + uuid.NewString()
		raw, _ := json.Marshal(map[string]any{"value": in.Amount, "precision": 20})
		if tx.Exec("INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,created_at,updated_at) VALUES(?,?,?,?,?,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, set, in.CurrencyCode, in.Amount, string(raw)).Error != nil {
			return ErrSchema
		}
		if tx.Exec("UPDATE catalog_price_edit_runtime SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrSchema
		}
		rt.Revision++
		after, e := loadPriceCreate(tx, p, v, rt)
		if e != nil {
			return e
		}
		out = PriceCreateResult{id, after.Revision, false}
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(after)
		return tx.Table("catalog_price_edits").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "product_id": p, "variant_id": v, "price_id": id, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_revision": out.Revision, "created_at": time.Now().UTC()}).Error
	})
	return out, e
}
func (h Handler) PriceCreateHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	if c.Request.URL.RawQuery != "" {
		priceEditFail(c, ErrPriceInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	p, v := c.Param("id"), c.Param("variant_id")
	if c.Request.Method == "GET" {
		out, e := h.ReadPriceCreate(ctx, actor, p, v)
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
	in, e := decodePriceCreate(c.Request.Body)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	out, e := h.CreateBasePrice(ctx, actor, p, v, in)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	c.JSON(200, out)
}
