package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type VariantCreateInput struct {
	OperationID      string   `json:"operation_id"`
	ExpectedRevision string   `json:"expected_revision"`
	Title            string   `json:"title"`
	SKU              string   `json:"sku"`
	OptionValueIDs   []string `json:"option_value_ids"`
}
type VariantCreateContext struct {
	ProductID string        `json:"product_id"`
	Options   []AdminOption `json:"options"`
	Enabled   bool          `json:"enabled"`
	Revision  string        `json:"revision"`
}
type VariantCreateResult struct {
	ID              string `json:"id"`
	InventoryItemID string `json:"inventory_item_id"`
	SKU             string `json:"sku"`
	Revision        string `json:"revision"`
	Replayed        bool   `json:"replayed"`
}
type variantCreateRuntime struct {
	Version       int
	Enabled       bool
	Revision      uint64
	LastSkuNumber int64
}
type creationVariant struct {
	ID, Title string
	SKU       *string
	UpdatedAt time.Time
}
type creationOption struct{ VariantID, OptionValueID string }

func validVariantCreate(in VariantCreateInput) bool {
	id, idErr := uuid.Parse(in.OperationID)
	rev, revErr := hex.DecodeString(in.ExpectedRevision)
	if idErr != nil || id.Version() != 4 || id.String() != in.OperationID || revErr != nil || len(rev) != 32 || hex.EncodeToString(rev) != in.ExpectedRevision || !utf8.ValidString(in.Title) || strings.TrimSpace(in.Title) == "" || utf8.RuneCountInString(in.Title) > 500 || containsControl(in.Title) || !utf8.ValidString(in.SKU) || utf8.RuneCountInString(in.SKU) > 512 || containsControl(in.SKU) || in.OptionValueIDs == nil || len(in.OptionValueIDs) > 1000 {
		return false
	}
	seen := map[string]bool{}
	for _, id := range in.OptionValueIDs {
		if !validProductID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

// Read a bounded object, rejecting duplicate fields, nulls, unknown fields and
// invalid UTF-8 before the standard decoder can silently replace input.
func decodeVariantCreate(r io.Reader) (VariantCreateInput, error) {
	var in VariantCreateInput
	b, e := io.ReadAll(io.LimitReader(r, 262145))
	if e != nil || len(b) > 262144 || !utf8.Valid(b) {
		return in, ErrPriceInput
	}
	d := json.NewDecoder(bytes.NewReader(b))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return in, ErrPriceInput
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		tok, e = d.Token()
		key, ok := tok.(string)
		if e != nil || !ok || fields[key] != nil {
			return in, ErrPriceInput
		}
		switch key {
		case "operation_id", "expected_revision", "title", "sku", "option_value_ids":
		default:
			return in, ErrPriceInput
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return in, ErrPriceInput
		}
		fields[key] = raw
	}
	if tok, e = d.Token(); e != nil || tok != json.Delim('}') || len(fields) != 5 || d.Decode(new(any)) != io.EOF {
		return in, ErrPriceInput
	}
	b, _ = json.Marshal(fields)
	if json.Unmarshal(b, &in) != nil || !validVariantCreate(in) {
		return in, ErrPriceInput
	}
	return in, nil
}
func lockVariantCreate(tx *gorm.DB) (variantCreateRuntime, error) {
	var rt variantCreateRuntime
	e := tx.Table("catalog_variant_create_runtime").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=1").Take(&rt).Error
	if e != nil || rt.Version != 1 || rt.Revision == 0 || rt.Revision == math.MaxUint64 || rt.LastSkuNumber < 0 {
		return rt, ErrSchema
	}
	return rt, nil
}
func loadVariantCreate(tx *gorm.DB, p string, rt variantCreateRuntime) (VariantCreateContext, []creationVariant, []creationOption, error) {
	out := VariantCreateContext{ProductID: p, Enabled: rt.Enabled}
	product, e := detailBase(tx, p)
	if e != nil {
		return out, nil, nil, e
	}
	out.Options, e = productOptions(tx, p)
	if e != nil {
		return out, nil, nil, e
	}
	var variants []creationVariant
	if tx.Table("catalog_variants").Where("product_id=? AND deleted_at IS NULL", p).Order("id").Limit(10001).Scan(&variants).Error != nil || len(variants) > 10000 {
		return out, nil, nil, ErrSchema
	}
	var values []creationOption
	if tx.Table("catalog_variant_options vo").Select("vo.variant_id,vo.option_value_id").Joins("JOIN catalog_variants v ON v.id=vo.variant_id AND v.deleted_at IS NULL").Where("v.product_id=?", p).Order("vo.variant_id,vo.option_value_id").Limit(100001).Scan(&values).Error != nil || len(values) > 100000 {
		return out, nil, nil, ErrSchema
	}
	b, _ := json.Marshal([]any{product, out.Options, variants, values, rt.Revision})
	s := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(s[:])
	return out, variants, values, nil
}
func (h Handler) ReadVariantCreate(ctx context.Context, actor identity.User, p string) (VariantCreateContext, error) {
	var out VariantCreateContext
	if !validProductID(p) {
		return out, ErrPriceInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.ProductRead, authorization.ProductWrite); e != nil {
			return e
		}
		rt, e := lockVariantCreate(tx)
		if e != nil {
			return e
		}
		out, _, _, e = loadVariantCreate(tx, p, rt)
		return e
	})
	return out, e
}

var erpNumberPattern = regexp.MustCompile(`^ERP[0-9]{5,18}$`)

func reservedSKUNumber(s string) int64 {
	if !erpNumberPattern.MatchString(s) {
		return 0
	}
	n, e := strconv.ParseInt(s[3:], 10, 64)
	if e != nil || n <= 10000 {
		return 0
	}
	return n - 10000
}
func inventoryCreateTitle(product, variant string) string {
	product, variant = strings.TrimSpace(product), strings.TrimSpace(variant)
	if product == "" || variant == product || strings.HasPrefix(variant, product+" / ") {
		if variant != "" {
			return variant
		}
		return product
	}
	if variant != "" {
		return product + " / " + variant
	}
	return product
}
func (h Handler) CreateVariant(ctx context.Context, actor identity.User, p string, in VariantCreateInput) (VariantCreateResult, error) {
	var out VariantCreateResult
	if !validProductID(p) || !validVariantCreate(in) {
		return out, ErrPriceInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	request, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.ProductRead, authorization.ProductWrite); e != nil {
			return e
		}
		// All SKU creators share this row, including different products. It protects
		// global uniqueness and numbering without adding constraints to imported data.
		rt, e := lockVariantCreate(tx)
		if e != nil {
			return e
		}
		if !rt.Enabled {
			return ErrSchema
		}
		var product struct{ ID, Title string }
		if e = tx.Table("catalog_products").Select("id,title").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted_at IS NULL", p).Take(&product).Error; e != nil {
			return e
		}
		before, variants, values, e := loadVariantCreate(tx, p, rt)
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, ProductID, VariantID, InventoryItemID, SKU, RequestJSON, ResultRevision string }
		e = tx.Table("catalog_variant_creations").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.ProductID != p || receipt.RequestJSON != string(request) {
				return ErrEditConflict
			}
			var count int64
			if tx.Table("catalog_variants v").Joins("JOIN catalog_variant_inventory_items vi ON vi.variant_id=v.id AND vi.deleted_at IS NULL").Joins("JOIN inventory_items i ON i.id=vi.inventory_item_id AND i.deleted_at IS NULL").Where("v.id=? AND v.product_id=? AND v.deleted_at IS NULL AND i.id=?", receipt.VariantID, p, receipt.InventoryItemID).Count(&count).Error != nil || count != 1 {
				return gorm.ErrRecordNotFound
			}
			out = VariantCreateResult{receipt.VariantID, receipt.InventoryItemID, receipt.SKU, receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		selected := map[string]bool{}
		for _, id := range in.OptionValueIDs {
			selected[id] = true
		}
		if len(selected) != len(before.Options) {
			return ErrPriceInput
		}
		for _, option := range before.Options {
			n := 0
			for _, v := range option.Values {
				if selected[v.ID] {
					n++
				}
			}
			if n != 1 {
				return ErrPriceInput
			}
		}
		// Compare complete sets, never just titles or a subset of option dimensions.
		existing := map[string][]string{}
		for _, v := range variants {
			existing[v.ID] = []string{}
		}
		for _, v := range values {
			existing[v.VariantID] = append(existing[v.VariantID], v.OptionValueID)
		}
		ids := append([]string{}, in.OptionValueIDs...)
		sort.Strings(ids)
		for _, set := range existing {
			sort.Strings(set)
			if len(set) == len(ids) {
				same := true
				for i := range set {
					if set[i] != ids[i] {
						same = false
						break
					}
				}
				if same {
					return ErrEditConflict
				}
			}
		}
		// Synchronize the persistent high water mark with imported and deleted ERP
		// codes. Explicit ERP codes reserve their number exactly as in the source.
		var max struct{ N int64 }
		if tx.Raw("SELECT COALESCE(MAX(CAST(SUBSTRING(sku,4) AS SIGNED)-10000),0) AS n FROM (SELECT sku FROM catalog_variants UNION ALL SELECT sku FROM inventory_items) codes WHERE sku REGEXP '^ERP[0-9]{5,18}$'").Scan(&max).Error != nil {
			return ErrSchema
		}
		if max.N > rt.LastSkuNumber {
			rt.LastSkuNumber = max.N
		}
		sku := in.SKU
		if strings.TrimSpace(sku) == "" {
			if rt.LastSkuNumber >= math.MaxInt64-10000 {
				return ErrSchema
			}
			rt.LastSkuNumber++
			sku = "ERP" + strconv.FormatInt(rt.LastSkuNumber+10000, 10)
		}
		if n := reservedSKUNumber(sku); n > rt.LastSkuNumber {
			rt.LastSkuNumber = n
		}
		for _, table := range []string{"catalog_variants", "inventory_items"} {
			var n int64
			if tx.Table(table).Where("sku=? AND deleted_at IS NULL", sku).Count(&n).Error != nil {
				return ErrSchema
			}
			if n > 0 {
				return ErrEditConflict
			}
		}
		id, item := "variant_"+uuid.NewString(), "iitem_"+uuid.NewString()
		if tx.Exec("INSERT INTO catalog_variants(id,product_id,title,sku,manage_inventory,allow_backorder,created_at,updated_at) VALUES(?,?,?,?,TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, p, in.Title, sku).Error != nil {
			return ErrSchema
		}
		for _, value := range ids {
			if tx.Exec("INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES(?,?)", id, value).Error != nil {
				return ErrSchema
			}
		}
		if tx.Exec("INSERT INTO inventory_items(id,sku,title,requires_shipping,created_at,updated_at) VALUES(?,?,?,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", item, sku, inventoryCreateTitle(product.Title, in.Title)).Error != nil {
			return ErrSchema
		}
		if tx.Exec("INSERT INTO catalog_variant_inventory_items(id,variant_id,inventory_item_id,required_quantity,created_at,updated_at) VALUES(?,?,?,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "pvii_"+uuid.NewString(), id, item).Error != nil {
			return ErrSchema
		}
		if tx.Exec("UPDATE catalog_variant_create_runtime SET revision=revision+1,last_sku_number=? WHERE id=1", rt.LastSkuNumber).Error != nil {
			return ErrSchema
		}
		rt.Revision++
		after, _, _, e := loadVariantCreate(tx, p, rt)
		if e != nil {
			return e
		}
		out = VariantCreateResult{id, item, sku, after.Revision, false}
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(out)
		return tx.Table("catalog_variant_creations").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "product_id": p, "variant_id": id, "inventory_item_id": item, "sku": sku, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_revision": out.Revision, "created_at": time.Now().UTC()}).Error
	})
	return out, e
}
func (h Handler) VariantCreateHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	fail := func(e error) {
		switch {
		case errors.Is(e, ErrPriceInput):
			platform.Fail(c, 400, "catalog.invalid_variant", "Invalid SKU configuration")
		case errors.Is(e, authorization.ErrDenied):
			platform.Fail(c, 403, "authorization.denied", "Current product permission required")
		case errors.Is(e, gorm.ErrRecordNotFound):
			platform.Fail(c, 404, "catalog.not_found", "Product or created SKU not found")
		case errors.Is(e, ErrEditConflict):
			platform.Fail(c, 409, "catalog.edit_conflict", "SKU or option configuration changed; reload")
		default:
			platform.Fail(c, 503, "catalog.unavailable", "SKU creation unavailable; retry the same operation")
		}
	}
	if c.Request.URL.RawQuery != "" {
		fail(ErrPriceInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if c.Request.Method == "GET" {
		out, e := h.ReadVariantCreate(ctx, actor, c.Param("id"))
		if e != nil {
			fail(e)
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
	in, e := decodeVariantCreate(c.Request.Body)
	if e != nil {
		fail(e)
		return
	}
	out, e := h.CreateVariant(ctx, actor, c.Param("id"), in)
	if e != nil {
		fail(e)
		return
	}
	c.JSON(200, out)
}
