package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProductCreateOption struct {
	Title  string   `json:"title"`
	Values []string `json:"values"`
}
type ProductCreateInput struct {
	OperationID       string                `json:"operation_id"`
	Title             string                `json:"title"`
	Handle            string                `json:"handle"`
	Subtitle          *string               `json:"subtitle"`
	Description       *string               `json:"description"`
	Status            string                `json:"status"`
	SalesChannelIDs   []string              `json:"sales_channel_ids"`
	ShippingProfileID string                `json:"shipping_profile_id"`
	CollectionID      *string               `json:"collection_id"`
	TypeID            *string               `json:"type_id"`
	CategoryIDs       []string              `json:"category_ids"`
	ImageURLs         []string              `json:"image_urls"`
	Options           []ProductCreateOption `json:"options"`
}
type ProductCreateChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type ProductCreateContext struct {
	Enabled          bool                  `json:"enabled"`
	Channels         []ProductCreateChoice `json:"channels"`
	ShippingProfiles []ProductCreateChoice `json:"shipping_profiles"`
	Collections      []ProductCreateChoice `json:"collections"`
	Types            []ProductCreateChoice `json:"types"`
	Categories       []ProductCreateChoice `json:"categories"`
}
type ProductCreateResult struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}

func createText(s string, max int) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max && !containsControl(s)
}
func validProductCreate(in ProductCreateInput) bool {
	// Reuse exact basic-field/status/operation validation, without accepting a
	// caller-provided existing product revision for a newly allocated identity.
	if validateEdit(EditInput{OperationID: in.OperationID, ExpectedRevision: strings.Repeat("0", 64), Title: in.Title, Subtitle: in.Subtitle, Description: in.Description, Status: in.Status}) != nil || !createText(in.Title, 500) || !createText(in.Handle, 512) || strings.TrimSpace(in.Handle) != in.Handle || strings.ContainsAny(in.Handle, "/?#\\") || in.SalesChannelIDs == nil || len(in.SalesChannelIDs) == 0 || len(in.SalesChannelIDs) > 20 || !validProductID(in.ShippingProfileID) || in.CategoryIDs == nil || len(in.CategoryIDs) > 20 || in.ImageURLs == nil || len(in.ImageURLs) > 20 || in.Options == nil || len(in.Options) > 8 {
		return false
	}
	for _, list := range [][]string{in.SalesChannelIDs, in.CategoryIDs} {
		seen := map[string]bool{}
		for _, id := range list {
			if !validProductID(id) || seen[id] {
				return false
			}
			seen[id] = true
		}
	}
	for _, id := range []*string{in.CollectionID, in.TypeID} {
		if id != nil && !validProductID(*id) {
			return false
		}
	}
	seenImages := map[string]bool{}
	for _, image := range in.ImageURLs {
		u, e := url.Parse(image)
		if e != nil || len(image) > 2048 || !utf8.ValidString(image) || containsControl(image) || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || seenImages[image] {
			return false
		}
		seenImages[image] = true
	}
	titles := map[string]bool{}
	total := 0
	for _, option := range in.Options {
		if !createText(option.Title, 100) || strings.TrimSpace(option.Title) != option.Title || titles[option.Title] || option.Values == nil || len(option.Values) == 0 || len(option.Values) > 100 {
			return false
		}
		titles[option.Title] = true
		values := map[string]bool{}
		for _, value := range option.Values {
			if !createText(value, 500) || strings.TrimSpace(value) != value || values[value] {
				return false
			}
			values[value] = true
			total++
		}
	}
	return total <= 500
}

// Decode the outer object and each option explicitly so duplicate JSON keys
// cannot be silently replaced before the immutable idempotency receipt.
func decodeProductCreate(r io.Reader) (ProductCreateInput, error) {
	var in ProductCreateInput
	raw, e := io.ReadAll(io.LimitReader(r, 262145))
	if e != nil || len(raw) > 262144 || !utf8.Valid(raw) {
		return in, errEditInput
	}
	object := func(raw []byte, allowed map[string]bool) (map[string]json.RawMessage, error) {
		d := json.NewDecoder(bytes.NewReader(raw))
		tok, e := d.Token()
		if e != nil || tok != json.Delim('{') {
			return nil, errEditInput
		}
		fields := map[string]json.RawMessage{}
		for d.More() {
			tok, e = d.Token()
			name, ok := tok.(string)
			if e != nil || !ok || !allowed[name] || fields[name] != nil {
				return nil, errEditInput
			}
			var value json.RawMessage
			if d.Decode(&value) != nil {
				return nil, errEditInput
			}
			fields[name] = value
		}
		if tok, e = d.Token(); e != nil || tok != json.Delim('}') || len(fields) != len(allowed) || d.Decode(new(any)) != io.EOF {
			return nil, errEditInput
		}
		return fields, nil
	}
	allowed := map[string]bool{}
	for _, name := range []string{"operation_id", "title", "handle", "subtitle", "description", "status", "sales_channel_ids", "shipping_profile_id", "collection_id", "type_id", "category_ids", "image_urls", "options"} {
		allowed[name] = true
	}
	fields, e := object(raw, allowed)
	if e != nil {
		return in, e
	}
	for name, value := range fields {
		if name != "subtitle" && name != "description" && name != "collection_id" && name != "type_id" && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return in, errEditInput
		}
	}
	var options []json.RawMessage
	if json.Unmarshal(fields["options"], &options) != nil {
		return in, errEditInput
	}
	for _, raw := range options {
		if _, e = object(raw, map[string]bool{"title": true, "values": true}); e != nil {
			return in, e
		}
	}
	if json.Unmarshal(raw, &in) != nil || !validProductCreate(in) {
		return in, errEditInput
	}
	return in, nil
}

func lockProductCreation(tx *gorm.DB) (bool, error) {
	var capability struct {
		Version int
		Enabled bool
	}
	if tx.Table("catalog_command_capabilities").Where("command=?", "product.create").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&capability).Error != nil || capability.Version != 1 {
		return false, ErrSchema
	}
	return capability.Enabled, nil
}

func productCreateChoices(tx *gorm.DB, table, label, condition string) ([]ProductCreateChoice, error) {
	out := []ProductCreateChoice{}
	if tx.Table(table).Select("id,"+label+" AS name").Where(condition).Order("id").Limit(1001).Scan(&out).Error != nil || len(out) > 1000 {
		return nil, ErrSchema
	}
	return out, nil
}
func (h Handler) ReadProductCreate(ctx context.Context, actor identity.User) (ProductCreateContext, error) {
	var out ProductCreateContext
	if h.DB == nil {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.ProductRead, authorization.ProductWrite); e != nil {
			return e
		}
		if e := Ready(ctx, tx); e != nil {
			return e
		}
		enabled, e := lockProductCreation(tx)
		if e != nil {
			return e
		}
		out.Enabled = enabled
		for _, source := range []struct {
			table, label, condition string
			dest                    *[]ProductCreateChoice
		}{
			{"commerce_sales_channels", "name", "deleted_at IS NULL AND is_disabled=FALSE", &out.Channels},
			{"fulfillment_shipping_profiles", "name", "deleted_at IS NULL", &out.ShippingProfiles},
			{"catalog_collections", "title", "deleted_at IS NULL", &out.Collections},
			{"catalog_product_types", "value", "deleted_at IS NULL", &out.Types},
			{"catalog_categories", "name", "deleted_at IS NULL AND is_active=TRUE AND is_internal=FALSE", &out.Categories},
		} {
			values, e := productCreateChoices(tx, source.table, source.label, source.condition)
			if e != nil {
				return e
			}
			*source.dest = values
		}
		return nil
	})
	return out, e
}
func (h Handler) CreateProduct(ctx context.Context, actor identity.User, in ProductCreateInput) (ProductCreateResult, error) {
	var out ProductCreateResult
	if !validProductCreate(in) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	request, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.ProductRead, authorization.ProductWrite); e != nil {
			return e
		}
		if e := Ready(ctx, tx); e != nil {
			return e
		}
		enabled, e := lockProductCreation(tx)
		if e != nil {
			return e
		}
		var old struct{ ActorID, ProductID, RequestJSON, ResultRevision string }
		e = tx.Table("catalog_product_edits").Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&old).Error
		if e == nil {
			if old.ActorID != actor.ID || old.RequestJSON != string(request) {
				return ErrEditConflict
			}
			var found struct{ ID string }
			if tx.Table("catalog_products").Select("id").Where("id=? AND deleted_at IS NULL", old.ProductID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&found).Error != nil {
				return ErrEditConflict
			}
			out = ProductCreateResult{old.ProductID, old.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !enabled {
			return ErrSchema
		}
		// Lock every current directory parent. These fixed names are not caller SQL.
		check := func(table, where, id string) error {
			var parent struct{ ID string }
			if e := tx.Table(table).Select("id").Where("id=? AND "+where, id).Clauses(clause.Locking{Strength: "SHARE"}).Take(&parent).Error; e != nil {
				if errors.Is(e, gorm.ErrRecordNotFound) {
					return errEditInput
				}
				return ErrSchema
			}
			return nil
		}
		for _, id := range in.SalesChannelIDs {
			if e := check("commerce_sales_channels", "deleted_at IS NULL AND is_disabled=FALSE", id); e != nil {
				return e
			}
		}
		if e := check("fulfillment_shipping_profiles", "deleted_at IS NULL", in.ShippingProfileID); e != nil {
			return e
		}
		for _, v := range []struct {
			table string
			id    *string
		}{{"catalog_collections", in.CollectionID}, {"catalog_product_types", in.TypeID}} {
			if v.id != nil {
				if e := check(v.table, "deleted_at IS NULL", *v.id); e != nil {
					return e
				}
			}
		}
		for _, id := range in.CategoryIDs {
			if e := check("catalog_categories", "deleted_at IS NULL AND is_active=TRUE AND is_internal=FALSE", id); e != nil {
				return e
			}
		}
		id := "prod_" + uuid.NewString()
		now := time.Now().UTC().Truncate(time.Microsecond)
		var thumbnail *string
		if len(in.ImageURLs) > 0 {
			thumbnail = &in.ImageURLs[0]
		}
		product := map[string]any{"id": id, "title": in.Title, "handle": in.Handle, "subtitle": in.Subtitle, "description": in.Description, "status": in.Status, "collection_id": in.CollectionID, "type_id": in.TypeID, "thumbnail": thumbnail, "is_giftcard": false, "discountable": true, "created_at": now, "updated_at": now}
		if e := tx.Table("catalog_products").Create(product).Error; e != nil {
			var duplicate *driver.MySQLError
			if errors.As(e, &duplicate) && duplicate.Number == 1062 {
				return ErrEditConflict
			}
			return e
		}
		for _, channel := range in.SalesChannelIDs {
			if e := tx.Exec("INSERT INTO catalog_product_channels(id,product_id,sales_channel_id,created_at,updated_at) VALUES(?,?,?,?,?)", "prodsc_"+uuid.NewString(), id, channel, now, now).Error; e != nil {
				return e
			}
		}
		if e := tx.Exec("INSERT INTO catalog_product_shipping_profiles(id,product_id,shipping_profile_id,created_at,updated_at) VALUES(?,?,?,?,?)", "prodsp_"+uuid.NewString(), id, in.ShippingProfileID, now, now).Error; e != nil {
			return e
		}
		for _, category := range in.CategoryIDs {
			if e := tx.Table("catalog_category_products").Create(map[string]any{"product_id": id, "category_id": category}).Error; e != nil {
				return e
			}
		}
		for rank, image := range in.ImageURLs {
			if e := tx.Table("catalog_images").Create(map[string]any{"id": "img_" + uuid.NewString(), "url": image, "product_id": id, "rank": rank, "created_at": now, "updated_at": now}).Error; e != nil {
				return e
			}
		}
		for _, option := range in.Options {
			oid, link := "opt_"+uuid.NewString(), "prodopt_"+uuid.NewString()
			if e := tx.Table("catalog_options").Create(map[string]any{"id": oid, "title": option.Title, "is_exclusive": true, "created_at": now, "updated_at": now}).Error; e != nil {
				return e
			}
			if e := tx.Table("catalog_product_options").Create(map[string]any{"id": link, "product_id": id, "product_option_id": oid, "created_at": now, "updated_at": now}).Error; e != nil {
				return e
			}
			for rank, value := range option.Values {
				vid := "optval_" + uuid.NewString()
				if e := tx.Table("catalog_option_values").Create(map[string]any{"id": vid, "option_id": oid, "value": value, "rank": rank, "created_at": now, "updated_at": now}).Error; e != nil {
					return e
				}
				if e := tx.Table("catalog_product_option_values").Create(map[string]any{"id": "prodoptval_" + uuid.NewString(), "product_product_option_id": link, "product_option_value_id": vid, "created_at": now, "updated_at": now}).Error; e != nil {
					return e
				}
			}
		}
		after := EditableProduct{in.Title, in.Subtitle, in.Description, in.Status, now}
		out = ProductCreateResult{id, editRevision(after), false}
		saved, _ := json.Marshal(after)
		return tx.Table("catalog_product_edits").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "product_id": id, "request_json": string(request), "before_json": "null", "after_json": string(saved), "result_revision": out.Revision, "created_at": now}).Error
	})
	return out, e
}
