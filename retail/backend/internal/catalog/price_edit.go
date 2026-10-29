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
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/pricing"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PriceAmountInput struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	Amount           string `json:"amount"`
}
type PriceEditContext struct {
	EffectiveAmount string `json:"effective_amount"`
	rawAmount       string
	ProductID       string  `json:"product_id"`
	VariantID       string  `json:"variant_id"`
	ID              string  `json:"id"`
	PriceSetID      string  `json:"price_set_id"`
	CurrencyCode    string  `json:"currency_code"`
	Amount          string  `json:"amount"`
	MinQuantity     *string `json:"min_quantity"`
	MaxQuantity     *string `json:"max_quantity"`
	PriceListID     *string `json:"price_list_id"`
	SharedVariants  int     `json:"shared_variants"`
	Enabled         bool    `json:"enabled"`
	Revision        string  `json:"revision"`
}

var priceAmountPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,19})\.[0-9]{18}$`)
var ErrPriceInput = errors.New("invalid price amount edit")

func validPriceAmount(in PriceAmountInput) bool {
	id, e := uuid.Parse(in.OperationID)
	rev, re := hex.DecodeString(in.ExpectedRevision)
	return e == nil && id.Version() == 4 && id.String() == in.OperationID && re == nil && len(rev) == 32 && hex.EncodeToString(rev) == in.ExpectedRevision && priceAmountPattern.MatchString(in.Amount)
}
func decodePriceAmount(r io.Reader) (PriceAmountInput, error) {
	var in PriceAmountInput
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
		k, e := d.Token()
		name, ok := k.(string)
		if e != nil || !ok {
			return in, ErrPriceInput
		}
		if _, ok := fields[name]; ok {
			return in, ErrPriceInput
		}
		if name != "operation_id" && name != "expected_revision" && name != "amount" {
			return in, ErrPriceInput
		}
		var v json.RawMessage
		if d.Decode(&v) != nil || len(v) == 0 || v[0] != '"' {
			return in, ErrPriceInput
		}
		var s string
		if json.Unmarshal(v, &s) != nil {
			return in, ErrPriceInput
		}
		fields[name] = s
	}
	if token, e = d.Token(); e != nil || token != json.Delim('}') || len(fields) != 3 || d.Decode(new(any)) != io.EOF {
		return in, ErrPriceInput
	}
	in = PriceAmountInput{fields["operation_id"], fields["expected_revision"], fields["amount"]}
	if !validPriceAmount(in) {
		return in, ErrPriceInput
	}
	return in, nil
}

type priceEditRuntime struct {
	Version  int
	Enabled  bool
	Revision uint64
}

// All future price/rule/list/membership writers must acquire this gate before
// reading configuration. Public quote readers see either committed snapshot.
func lockPriceEdit(tx *gorm.DB) (priceEditRuntime, error) {
	var rt priceEditRuntime
	e := tx.Table("catalog_price_edit_runtime").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=1").Take(&rt).Error
	if e != nil || rt.Version != 1 || rt.Revision == 0 || rt.Revision == ^uint64(0) {
		return rt, ErrSchema
	}
	return rt, nil
}

func loadPriceEdit(tx *gorm.DB, productID, variantID, priceID string, rt priceEditRuntime) (PriceEditContext, error) {
	out := PriceEditContext{ProductID: productID, VariantID: variantID, Enabled: rt.Enabled}
	var row struct {
		ID, PriceSetID, CurrencyCode, Amount, RawAmount                       string
		MinQuantity, MaxQuantity, RawMinQuantity, RawMaxQuantity, PriceListID *string
		RulesCount                                                            *int
		UpdatedAt                                                             time.Time
	}
	e := tx.Table("pricing_prices p").Select("p.id,p.price_set_id,p.currency_code,p.amount,p.raw_amount,p.min_quantity,p.max_quantity,p.raw_min_quantity,p.raw_max_quantity,p.price_list_id,p.rules_count,p.updated_at").Joins("JOIN pricing_price_sets ps ON ps.id=p.price_set_id AND ps.deleted_at IS NULL").Joins("JOIN catalog_variant_price_sets vp ON vp.price_set_id=p.price_set_id AND vp.deleted_at IS NULL").Joins("JOIN catalog_variants v ON v.id=vp.variant_id AND v.deleted_at IS NULL").Joins("JOIN catalog_products cp ON cp.id=v.product_id AND cp.deleted_at IS NULL").Where("p.id=? AND p.deleted_at IS NULL AND v.id=? AND cp.id=?", priceID, variantID, productID).Take(&row).Error
	if e != nil {
		return out, e
	}
	// Include full active rule/list context and exact membership in the revision,
	// even though this command may change only amount and its BigNumber mirror.
	var rules []struct {
		ID, Attribute, Operator, Value string
		Priority                       int
		UpdatedAt                      time.Time
	}
	if tx.Table("pricing_price_rules").Where("price_id=? AND deleted_at IS NULL", priceID).Order("id").Limit(10001).Scan(&rules).Error != nil || len(rules) > 10000 {
		return out, ErrSchema
	}
	var list []struct {
		ID, Title, Status, Type string
		StartsAt, EndsAt        *time.Time
		RulesCount              *int
		UpdatedAt               time.Time
	}
	var listRules []struct {
		ID, Attribute string
		Value         *string
		UpdatedAt     time.Time
	}
	if row.PriceListID != nil {
		if tx.Table("pricing_price_lists").Where("id=? AND deleted_at IS NULL", *row.PriceListID).Scan(&list).Error != nil {
			return out, ErrSchema
		}
		if len(list) > 0 && (tx.Table("pricing_price_list_rules").Where("price_list_id=? AND deleted_at IS NULL", *row.PriceListID).Order("id").Limit(10001).Scan(&listRules).Error != nil || len(listRules) > 10000) {
			return out, ErrSchema
		}
	}
	var members []struct{ ID, VariantID, ProductID string }
	if tx.Table("catalog_variant_price_sets vp").Select("vp.id,vp.variant_id,v.product_id").Joins("JOIN catalog_variants v ON v.id=vp.variant_id AND v.deleted_at IS NULL").Joins("JOIN catalog_products p ON p.id=v.product_id AND p.deleted_at IS NULL").Where("vp.price_set_id=? AND vp.deleted_at IS NULL", row.PriceSetID).Order("vp.id").Limit(10001).Scan(&members).Error != nil || len(members) > 10000 {
		return out, ErrSchema
	}
	seen := map[string]bool{}
	for _, m := range members {
		seen[m.VariantID] = true
	}
	effective, e := pricing.ConfiguredAmount(row.Amount, row.RawAmount)
	if e != nil {
		return out, ErrSchema
	}
	out.EffectiveAmount = effective
	out.rawAmount = row.RawAmount
	out.ID = row.ID
	out.PriceSetID = row.PriceSetID
	out.CurrencyCode = row.CurrencyCode
	out.Amount = row.Amount
	out.MinQuantity = row.MinQuantity
	out.MaxQuantity = row.MaxQuantity
	out.PriceListID = row.PriceListID
	out.SharedVariants = len(seen)
	b, _ := json.Marshal([]any{productID, variantID, row, rules, list, listRules, members, rt.Revision})
	sum := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(sum[:])
	return out, nil
}
func validPricePath(productID, variantID, priceID string) bool {
	return validProductID(productID) && validProductID(variantID) && validProductID(priceID)
}
func (h Handler) ReadPriceEdit(ctx context.Context, actor identity.User, productID, variantID, priceID string) (PriceEditContext, error) {
	var out PriceEditContext
	if !validPricePath(productID, variantID, priceID) {
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
		out, e = loadPriceEdit(tx, productID, variantID, priceID, rt)
		return e
	})
	return out, e
}
func (h Handler) EditPriceAmount(ctx context.Context, actor identity.User, productID, variantID, priceID string, in PriceAmountInput) (EditResult, error) {
	var out EditResult
	if !validPricePath(productID, variantID, priceID) || !validPriceAmount(in) {
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
		before, e := loadPriceEdit(tx, productID, variantID, priceID, rt)
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, ProductID, VariantID, PriceID, RequestJSON, ResultRevision string }
		e = tx.Table("catalog_price_edits").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.ProductID != productID || receipt.VariantID != variantID || receipt.PriceID != priceID || receipt.RequestJSON != string(request) {
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
		raw, _ := json.Marshal(map[string]any{"value": in.Amount, "precision": 20})
		if tx.Exec("UPDATE pricing_prices SET amount=?,raw_amount=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND deleted_at IS NULL", in.Amount, string(raw), priceID).Error != nil {
			return ErrSchema
		}
		if tx.Exec("UPDATE catalog_price_edit_runtime SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrSchema
		}
		rt.Revision++
		after, e := loadPriceEdit(tx, productID, variantID, priceID, rt)
		if e != nil {
			return e
		}
		out.Revision = after.Revision
		b, _ := json.Marshal(map[string]any{"price": before, "raw_amount": before.rawAmount})
		a, _ := json.Marshal(map[string]any{"price": after, "raw_amount": after.rawAmount})
		return tx.Table("catalog_price_edits").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "product_id": productID, "variant_id": variantID, "price_id": priceID, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_revision": out.Revision, "created_at": time.Now().UTC()}).Error
	})
	return out, e
}
func (h Handler) PriceEditHTTP(c *gin.Context) {
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
	p, v, id := c.Param("id"), c.Param("variant_id"), c.Param("price_id")
	if c.Request.Method == "GET" {
		out, e := h.ReadPriceEdit(ctx, actor, p, v, id)
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
	in, e := decodePriceAmount(c.Request.Body)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	out, e := h.EditPriceAmount(ctx, actor, p, v, id, in)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	c.JSON(200, out)
}
func priceEditFail(c *gin.Context, e error) {
	switch {
	case errors.Is(e, ErrPriceInput):
		platform.Fail(c, 400, "catalog.invalid_price_edit", "Invalid price edit")
	case errors.Is(e, authorization.ErrDenied):
		platform.Fail(c, 403, "authorization.denied", "Current price permission required")
	case errors.Is(e, gorm.ErrRecordNotFound):
		platform.Fail(c, 404, "catalog.not_found", "Product variant price not found")
	case errors.Is(e, ErrEditConflict):
		platform.Fail(c, 409, "catalog.edit_conflict", "Price configuration changed; reload")
	default:
		platform.Fail(c, 503, "catalog.unavailable", "Price edit unavailable; retry the same operation")
	}
}
