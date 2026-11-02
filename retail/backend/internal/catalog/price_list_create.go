package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/customergroups"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"mime"
	"net/url"
	"sort"
	"time"
	"unicode/utf8"
)

type InitialListPrice struct {
	ProductID    string `json:"product_id"`
	VariantID    string `json:"variant_id"`
	CurrencyCode string `json:"currency_code"`
	Amount       string `json:"amount"`
	RegionID     string `json:"region_id"`
	MinQuantity  string `json:"min_quantity"`
	MaxQuantity  string `json:"max_quantity"`
}
type PriceListCreateInput struct {
	PriceListInput
	ProductIDs       []string           `json:"product_ids"`
	Prices           []InitialListPrice `json:"prices"`
	CustomerGroupIDs []string           `json:"customer_group_ids"`
}
type PriceListCreateVariant struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	SKU            *string `json:"sku"`
	PriceSetID     *string `json:"price_set_id"`
	SharedVariants int     `json:"shared_variants"`
}
type PriceListCreateProduct struct {
	ID       string                   `json:"id"`
	Title    string                   `json:"title"`
	Variants []PriceListCreateVariant `json:"variants"`
}
type PriceListCreateContext struct {
	CustomerGroups []customergroups.Group   `json:"customer_groups"`
	Enabled        bool                     `json:"enabled"`
	Revision       string                   `json:"revision"`
	Products       []PriceListCreateProduct `json:"products"`
	Currencies     []string                 `json:"currencies"`
	Regions        []PriceRegion            `json:"regions"`
}
type PriceListCreateResult struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}

func validCreateListIDs(ids []string, min, max int) bool {
	if ids == nil || len(ids) < min || len(ids) > max {
		return false
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !validProductID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func validPriceListCreate(in PriceListCreateInput) bool {
	if !validPriceListInput(in.PriceListInput) || !validCreateListIDs(in.ProductIDs, 1, 20) || !validCreateListIDs(in.CustomerGroupIDs, 0, 100) || in.Prices == nil || len(in.Prices) > 200 {
		return false
	}
	selected := map[string]bool{}
	for _, id := range in.ProductIDs {
		selected[id] = true
	}
	for _, p := range in.Prices {
		if !selected[p.ProductID] || !validProductID(p.VariantID) || !validPriceCreate(PriceCreateInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, CurrencyCode: p.CurrencyCode, Amount: p.Amount, RegionID: p.RegionID, MinQuantity: p.MinQuantity, MaxQuantity: p.MaxQuantity}) {
			return false
		}
	}
	return true
}
func listCreateObject(raw []byte, names []string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return nil, ErrPriceInput
	}
	allowed := map[string]bool{}
	for _, n := range names {
		allowed[n] = true
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		tok, e = d.Token()
		k, ok := tok.(string)
		if e != nil || !ok || !allowed[k] || fields[k] != nil {
			return nil, ErrPriceInput
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil, ErrPriceInput
		}
		fields[k] = value
	}
	if tok, e = d.Token(); e != nil || tok != json.Delim('}') || len(fields) != len(names) || d.Decode(new(any)) != io.EOF {
		return nil, ErrPriceInput
	}
	return fields, nil
}
func decodePriceListCreate(r io.Reader) (PriceListCreateInput, error) {
	var in PriceListCreateInput
	raw, e := io.ReadAll(io.LimitReader(r, 262145))
	if e != nil || len(raw) > 262144 || !utf8.Valid(raw) {
		return in, ErrPriceInput
	}
	fields, e := listCreateObject(raw, []string{"operation_id", "expected_revision", "title", "description", "status", "type", "starts_at", "ends_at", "product_ids", "prices", "customer_group_ids"})
	if e != nil {
		return in, e
	}
	basic := map[string]json.RawMessage{}
	for _, k := range []string{"operation_id", "expected_revision", "title", "description", "status", "type", "starts_at", "ends_at"} {
		basic[k] = fields[k]
	}
	b, _ := json.Marshal(basic)
	in.PriceListInput, e = decodePriceListInput(bytes.NewReader(b))
	if e != nil {
		return in, e
	}
	if json.Unmarshal(fields["product_ids"], &in.ProductIDs) != nil || json.Unmarshal(fields["customer_group_ids"], &in.CustomerGroupIDs) != nil {
		return in, ErrPriceInput
	}
	var rows []json.RawMessage
	if json.Unmarshal(fields["prices"], &rows) != nil || rows == nil || len(rows) > 200 {
		return in, ErrPriceInput
	}
	in.Prices = []InitialListPrice{}
	for _, row := range rows {
		f, e := listCreateObject(row, []string{"product_id", "variant_id", "currency_code", "amount", "region_id", "min_quantity", "max_quantity"})
		if e != nil {
			return in, e
		}
		for _, v := range f {
			if len(v) == 0 || v[0] != '"' {
				return in, ErrPriceInput
			}
		}
		var p InitialListPrice
		if json.Unmarshal(row, &p) != nil {
			return in, ErrPriceInput
		}
		in.Prices = append(in.Prices, p)
	}
	if !validPriceListCreate(in) {
		return in, ErrPriceInput
	}
	return in, nil
}
func loadPriceListCreate(tx *gorm.DB, ids []string, rt priceEditRuntime) (PriceListCreateContext, error) {
	out := PriceListCreateContext{Enabled: rt.Enabled, Products: []PriceListCreateProduct{}, Currencies: []string{}, Regions: []PriceRegion{}}
	var err error
	out.CustomerGroups, err = customergroups.ActiveTx(tx)
	if err != nil {
		return out, err
	}
	if tx.Table("commerce_currencies").Where("deleted_at IS NULL AND code REGEXP '^[a-z]{3}$'").Order("code").Limit(1001).Pluck("code", &out.Currencies).Error != nil || len(out.Currencies) > 1000 {
		return out, ErrSchema
	}
	if tx.Table("commerce_regions r").Select("r.id,r.name,r.currency_code").Joins("JOIN commerce_currencies c ON c.code=r.currency_code AND c.deleted_at IS NULL").Where("r.deleted_at IS NULL").Order("r.id").Limit(1001).Scan(&out.Regions).Error != nil || len(out.Regions) > 1000 {
		return out, ErrSchema
	}
	sorted := append([]string{}, ids...)
	sort.Strings(sorted)
	versions := []any{rt.Revision, out.Currencies, out.Regions, out.CustomerGroups}
	total := 0
	for _, id := range sorted {
		var product struct {
			ID, Title string
			UpdatedAt time.Time
		}
		if e := tx.Table("catalog_products").Where("id=? AND deleted_at IS NULL", id).Take(&product).Error; e != nil {
			return out, e
		}
		var variants []struct {
			ID, Title string
			SKU       *string
			UpdatedAt time.Time
		}
		if e := tx.Table("catalog_variants").Where("product_id=? AND deleted_at IS NULL", id).Order("id").Limit(501).Scan(&variants).Error; e != nil {
			return out, e
		}
		total += len(variants)
		if total > 500 {
			return out, ErrPriceInput
		}
		p := PriceListCreateProduct{ID: product.ID, Title: product.Title, Variants: []PriceListCreateVariant{}}
		versions = append(versions, product, variants)
		for _, v := range variants {
			c, e := loadPriceCreate(tx, id, v.ID, rt)
			if e != nil {
				return out, e
			}
			p.Variants = append(p.Variants, PriceListCreateVariant{v.ID, v.Title, v.SKU, c.PriceSetID, c.SharedVariants})
			versions = append(versions, c.Revision)
		}
		out.Products = append(out.Products, p)
	}
	b, _ := json.Marshal(versions)
	sum := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(sum[:])
	return out, nil
}
func (h Handler) ReadPriceListCreate(ctx context.Context, actor identity.User, ids []string) (PriceListCreateContext, error) {
	var out PriceListCreateContext
	if !validCreateListIDs(ids, 0, 20) {
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
		out, e = loadPriceListCreate(tx, ids, rt)
		return e
	})
	return out, e
}

type savedListPrice struct {
	ID string `json:"id"`
	InitialListPrice
}

func insertListPrices(tx *gorm.DB, id string, prices []InitialListPrice, rt priceEditRuntime) ([]savedListPrice, error) {
	saved := []savedListPrice{}
	for _, p := range prices {
		c, e := loadPriceCreate(tx, p.ProductID, p.VariantID, rt)
		if e != nil {
			return nil, e
		}
		validCurrency := false
		for _, code := range c.Currencies {
			if code == p.CurrencyCode {
				validCurrency = true
			}
		}
		if !validCurrency {
			return nil, ErrPriceInput
		}
		if p.RegionID != "" {
			found := false
			for _, r := range c.Regions {
				if r.ID == p.RegionID && r.CurrencyCode == p.CurrencyCode {
					found = true
				}
			}
			if !found {
				return nil, ErrPriceInput
			}
		}
		for _, existing := range c.quantityPrices {
			if existing.PriceListID == id && existing.CurrencyCode == p.CurrencyCode && existing.RegionID == p.RegionID && existing.MinQuantity == p.MinQuantity && existing.MaxQuantity == p.MaxQuantity {
				return nil, ErrEditConflict
			}
		}
		command := PriceCreateInput{PriceListID: id, CurrencyCode: p.CurrencyCode, Amount: p.Amount, RegionID: p.RegionID, MinQuantity: p.MinQuantity, MaxQuantity: p.MaxQuantity}
		priceID, e := insertConfiguredPrice(tx, p.VariantID, command, c)
		if e != nil {
			return nil, e
		}
		saved = append(saved, savedListPrice{priceID, p})
	}
	return saved, nil
}
func (h Handler) CreatePriceList(ctx context.Context, actor identity.User, in PriceListCreateInput) (PriceListCreateResult, error) {
	var out PriceListCreateResult
	if !validPriceListCreate(in) {
		return out, ErrPriceInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	request, _ := json.Marshal(struct {
		Action string `json:"action"`
		PriceListCreateInput
	}{"create", in})
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
		before, e := loadPriceListCreate(tx, in.ProductIDs, rt)
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, PriceListID, RequestJSON, ResultRevision string }
		e = tx.Table("catalog_price_list_edits").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.RequestJSON != string(request) {
				return ErrEditConflict
			}
			if _, e := loadPriceList(tx, receipt.PriceListID, rt); e != nil {
				return e
			}
			for _, p := range in.Prices {
				var v struct{ ID string }
				if e := tx.Table("catalog_variants").Where("id=? AND product_id=? AND deleted_at IS NULL", p.VariantID, p.ProductID).Take(&v).Error; e != nil {
					return e
				}
			}
			out = PriceListCreateResult{receipt.PriceListID, receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		for _, id := range in.CustomerGroupIDs {
			found := false
			for _, group := range before.CustomerGroups {
				if group.ID == id {
					found = true
				}
			}
			if !found {
				return ErrPriceInput
			}
		}
		id := "plist_" + uuid.NewString()
		starts, _ := priceListTime(in.StartsAt)
		ends, _ := priceListTime(in.EndsAt)
		rules := 0
		if len(in.CustomerGroupIDs) > 0 {
			rules = 1
		}
		if e := tx.Exec("INSERT INTO pricing_price_lists(id,title,description,status,type,starts_at,ends_at,rules_count,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, in.Title, in.Description, in.Status, in.Type, starts, ends, rules).Error; e != nil {
			return e
		}
		if rules == 1 {
			value, _ := json.Marshal(in.CustomerGroupIDs)
			if e := tx.Exec("INSERT INTO pricing_price_list_rules(id,price_list_id,attribute,value,created_at,updated_at) VALUES(?,?,'customer.groups.id',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "plrule_"+uuid.NewString(), id, string(value)).Error; e != nil {
				return e
			}
		}
		saved, e := insertListPrices(tx, id, in.Prices, rt)
		if e != nil {
			return e
		}
		if e := tx.Exec("UPDATE catalog_price_edit_runtime SET revision=revision+1 WHERE id=1").Error; e != nil {
			return e
		}
		rt.Revision++
		detail, e := loadPriceList(tx, id, rt)
		if e != nil {
			return e
		}
		out = PriceListCreateResult{id, detail.Revision, false}
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(struct {
			PriceListDetail
			Prices []savedListPrice `json:"prices"`
		}{detail, saved})
		return tx.Table("catalog_price_list_edits").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "price_list_id": id, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_revision": out.Revision, "created_at": time.Now().UTC()}).Error
	})
	return out, e
}
func (h Handler) PriceListCreateHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
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
		out, e := h.ReadPriceListCreate(ctx, actor, ids)
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
	in, e := decodePriceListCreate(c.Request.Body)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	out, e := h.CreatePriceList(ctx, actor, in)
	if e != nil {
		priceEditFail(c, e)
		return
	}
	c.JSON(200, out)
}
