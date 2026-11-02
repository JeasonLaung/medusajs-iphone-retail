package catalog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrOptionInUse = errors.New("option used by active SKU")

type OptionSelection struct {
	ID     *string                `json:"id"`
	Title  string                 `json:"title"`
	Values []OptionSelectionValue `json:"values"`
}
type OptionSelectionValue struct {
	ID    *string `json:"id"`
	Value string  `json:"value"`
}
type ProductOptionsInput struct {
	OperationID      string            `json:"operation_id"`
	ExpectedRevision string            `json:"expected_revision"`
	Options          []OptionSelection `json:"options"`
}
type ManagedOptionValue struct {
	ID       string `json:"id"`
	Value    string `json:"value"`
	Rank     *int   `json:"rank"`
	Selected bool   `json:"selected"`
	Used     bool   `json:"used"`
}
type ManagedOption struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	IsExclusive bool                 `json:"is_exclusive"`
	Selected    bool                 `json:"selected"`
	Values      []ManagedOptionValue `json:"values"`
}
type ProductOptionsContext struct {
	ProductID string          `json:"product_id"`
	Enabled   bool            `json:"enabled"`
	Revision  string          `json:"revision"`
	Options   []ManagedOption `json:"options"`
}

func validProductOptions(in ProductOptionsInput) bool {
	// Share canonical operation/revision validation without accepting any image fields.
	if !validVariantImages(VariantImageInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, ImageIDs: []string{}}) || in.Options == nil || len(in.Options) > 8 {
		return false
	}
	ids, titles := map[string]bool{}, map[string]bool{}
	total := 0
	for _, o := range in.Options {
		if !createText(o.Title, 100) || strings.TrimSpace(o.Title) != o.Title || titles[o.Title] || o.Values == nil || len(o.Values) == 0 || len(o.Values) > 100 {
			return false
		}
		titles[o.Title] = true
		if o.ID != nil {
			if !validProductID(*o.ID) || ids[*o.ID] {
				return false
			}
			ids[*o.ID] = true
		}
		values, vids := map[string]bool{}, map[string]bool{}
		for _, v := range o.Values {
			if !createText(v.Value, 500) || strings.TrimSpace(v.Value) != v.Value || values[v.Value] {
				return false
			}
			values[v.Value] = true
			total++
			if v.ID != nil {
				if o.ID == nil || !validProductID(*v.ID) || vids[*v.ID] {
					return false
				}
				vids[*v.ID] = true
			}
		}
	}
	return total <= 500
}
func decodeProductOptions(r io.Reader) (ProductOptionsInput, error) {
	var in ProductOptionsInput
	raw, e := io.ReadAll(io.LimitReader(r, 262145))
	if e != nil {
		return in, e
	}
	if len(raw) > 262144 || !utf8.Valid(raw) {
		return in, errEditInput
	}
	fields, e := imageObject(raw, "operation_id", "expected_revision", "options")
	if e != nil {
		return in, e
	}
	for _, v := range fields {
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return in, errEditInput
		}
	}
	var options []json.RawMessage
	if json.Unmarshal(fields["options"], &options) != nil {
		return in, errEditInput
	}
	for _, o := range options {
		f, e := imageObject(o, "id", "title", "values")
		if e != nil {
			return in, e
		}
		var values []json.RawMessage
		if json.Unmarshal(f["values"], &values) != nil {
			return in, errEditInput
		}
		for _, v := range values {
			if _, e := imageObject(v, "id", "value"); e != nil {
				return in, e
			}
		}
	}
	if json.Unmarshal(raw, &in) != nil || !validProductOptions(in) {
		return in, errEditInput
	}
	return in, nil
}
func lockProductOptions(tx *gorm.DB, p string) (bool, error) {
	// Shared definition deletion and product membership changes must serialize.
	// The global gate controls definition writes only; its disabled state must
	// not disable the existing product-specific options command.
	if _, e := lockOptionDefinitions(tx, true); e != nil {
		return false, e
	}
	var cap struct {
		Version int
		Enabled bool
	}
	if tx.Table("catalog_command_capabilities").Where("command='product.options'").Clauses(clause.Locking{Strength: "SHARE"}).Take(&cap).Error != nil || cap.Version != 1 {
		return false, ErrSchema
	}
	var row struct{ ID string }
	if e := tx.Table("catalog_products").Select("id").Where("id=? AND deleted_at IS NULL", p).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error; e != nil {
		return false, e
	}
	return cap.Enabled, nil
}

// Directory contains shared definitions plus this product's exclusive options.
// Global definitions are never renamed, deleted or extended by product editing.
func loadProductOptions(tx *gorm.DB, p string, enabled bool) (ProductOptionsContext, error) {
	out := ProductOptionsContext{ProductID: p, Enabled: enabled, Options: []ManagedOption{}}
	var rows []struct {
		ID, Title   string
		IsExclusive bool
	}
	if tx.Table("catalog_options o").Select("o.id,o.title,o.is_exclusive").Where("o.deleted_at IS NULL AND (o.is_exclusive=FALSE OR EXISTS(SELECT 1 FROM catalog_product_options po WHERE po.product_id=? AND po.product_option_id=o.id AND po.deleted_at IS NULL))", p).Order("o.title,o.id").Limit(1001).Scan(&rows).Error != nil || len(rows) > 1000 {
		return out, ErrSchema
	}
	var links []struct{ ID, ProductOptionID string }
	if tx.Table("catalog_product_options").Select("id,product_option_id").Where("product_id=? AND deleted_at IS NULL", p).Order("id").Limit(1001).Scan(&links).Error != nil || len(links) > 1000 {
		return out, ErrSchema
	}
	selected := map[string]string{}
	linkIDs := []string{}
	for _, l := range links {
		if selected[l.ProductOptionID] != "" {
			return out, ErrSchema
		}
		selected[l.ProductOptionID] = l.ID
		linkIDs = append(linkIDs, l.ID)
	}
	var members []struct{ ID, ProductProductOptionID, ProductOptionValueID string }
	if len(linkIDs) > 0 {
		if tx.Table("catalog_product_option_values").Select("id,product_product_option_id,product_option_value_id").Where("product_product_option_id IN ? AND deleted_at IS NULL", linkIDs).Order("id").Limit(10001).Scan(&members).Error != nil || len(members) > 10000 {
			return out, ErrSchema
		}
	}
	membersByLink := map[string]map[string]bool{}
	for _, m := range members {
		if membersByLink[m.ProductProductOptionID] == nil {
			membersByLink[m.ProductProductOptionID] = map[string]bool{}
		}
		if membersByLink[m.ProductProductOptionID][m.ProductOptionValueID] {
			return out, ErrSchema
		}
		membersByLink[m.ProductProductOptionID][m.ProductOptionValueID] = true
	}
	var used []string
	if tx.Table("catalog_variant_options vo").Select("DISTINCT vo.option_value_id").Joins("JOIN catalog_variants v ON v.id=vo.variant_id AND v.deleted_at IS NULL").Where("v.product_id=?", p).Order("vo.option_value_id").Limit(10001).Pluck("vo.option_value_id", &used).Error != nil || len(used) > 10000 {
		return out, ErrSchema
	}
	usage := map[string]bool{}
	for _, v := range used {
		usage[v] = true
	}
	ids := []string{}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	var vals []struct {
		ID, OptionID, Value string
		Rank                *int
	}
	if len(ids) > 0 {
		if tx.Table("catalog_option_values").Select("id,option_id,value,`rank`").Where("option_id IN ? AND deleted_at IS NULL", ids).Order("`rank` IS NULL,`rank`,id").Limit(10001).Scan(&vals).Error != nil || len(vals) > 10000 {
			return out, ErrSchema
		}
	}
	found := map[string]bool{}
	foundMembers := 0
	for _, r := range rows {
		o := ManagedOption{ID: r.ID, Title: r.Title, IsExclusive: r.IsExclusive, Selected: selected[r.ID] != "", Values: []ManagedOptionValue{}}
		found[r.ID] = true
		for _, v := range vals {
			if v.OptionID == r.ID {
				sel := membersByLink[selected[r.ID]][v.ID]
				if sel {
					foundMembers++
				}
				o.Values = append(o.Values, ManagedOptionValue{v.ID, v.Value, v.Rank, sel, usage[v.ID]})
			}
		}
		out.Options = append(out.Options, o)
	}
	for id := range selected {
		if !found[id] {
			return out, ErrSchema
		}
	}
	if foundMembers != len(members) {
		return out, ErrSchema
	}
	out.Revision = imageDigest(struct {
		Options []ManagedOption
		Links   []struct{ ID, ProductOptionID string }
		Members []struct{ ID, ProductProductOptionID, ProductOptionValueID string }
		Used    []string
	}{out.Options, links, members, used})
	return out, nil
}
func (h Handler) ReadProductOptions(ctx context.Context, p string, actor identity.User) (ProductOptionsContext, error) {
	var out ProductOptionsContext
	if !validProductID(p) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := imageAccess(ctx, tx, actor); e != nil {
			return e
		}
		enabled, e := lockProductOptions(tx, p)
		if e != nil {
			return e
		}
		out, e = loadProductOptions(tx, p, enabled)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}

func (h Handler) EditProductOptions(ctx context.Context, p string, actor identity.User, in ProductOptionsInput) (ImageResult, error) {
	var out ImageResult
	if !validProductID(p) || !validProductOptions(in) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	requestSHA := imageDigest(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := imageAccess(ctx, tx, actor); e != nil {
			return e
		}
		enabled, e := lockProductOptions(tx, p)
		if e != nil {
			return e
		}
		var receipt struct{ ProductID, ActorID, RequestSHA256, ResultRevision string }
		e = tx.Table("catalog_option_edits").Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&receipt).Error
		if e == nil {
			if receipt.ProductID != p || receipt.ActorID != actor.ID || receipt.RequestSHA256 != requestSHA {
				return ErrEditConflict
			}
			out = ImageResult{receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !enabled {
			return ErrSchema
		}
		before, e := loadProductOptions(tx, p, enabled)
		if e != nil {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		current := map[string]ManagedOption{}
		for _, o := range before.Options {
			current[o.ID] = o
		}
		desired := map[string]map[string]bool{}
		// Validate the entire command before writing any definitions or memberships.
		for _, o := range in.Options {
			if o.ID == nil {
				continue
			}
			old, ok := current[*o.ID]
			if !ok || old.Title != o.Title {
				return errEditInput
			}
			allowed := map[string]ManagedOptionValue{}
			byText := map[string]string{}
			for _, v := range old.Values {
				allowed[v.ID] = v
				byText[v.Value] = v.ID
			}
			desired[old.ID] = map[string]bool{}
			for _, v := range o.Values {
				if v.ID == nil {
					if !old.IsExclusive || byText[v.Value] != "" {
						return errEditInput
					}
					continue
				}
				value, ok := allowed[*v.ID]
				if !ok || value.Value != v.Value {
					return errEditInput
				}
				desired[old.ID][*v.ID] = true
			}
		}
		for _, old := range before.Options {
			if old.Selected {
				for _, v := range old.Values {
					if v.Selected && v.Used && !desired[old.ID][v.ID] {
						return ErrOptionInUse
					}
				}
			}
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		// Detach removed groups; retire only orphaned exclusive definitions.
		for _, old := range before.Options {
			if old.Selected && desired[old.ID] == nil {
				var link struct{ ID string }
				if e := tx.Table("catalog_product_options").Select("id").Where("product_id=? AND product_option_id=? AND deleted_at IS NULL", p, old.ID).Take(&link).Error; e != nil {
					return e
				}
				if e := tx.Table("catalog_product_option_values").Where("product_product_option_id=? AND deleted_at IS NULL", link.ID).Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; e != nil {
					return e
				}
				if e := tx.Table("catalog_product_options").Where("id=?", link.ID).Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; e != nil {
					return e
				}
				if old.IsExclusive {
					var n int64
					if e := tx.Table("catalog_product_options").Where("product_option_id=? AND deleted_at IS NULL", old.ID).Count(&n).Error; e != nil {
						return e
					}
					if n == 0 {
						if e := tx.Table("catalog_options").Where("id=?", old.ID).Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; e != nil {
							return e
						}
					}
				}
			}
		}
		for _, o := range in.Options {
			oid := ""
			newGroup := o.ID == nil
			if newGroup {
				oid = "opt_" + uuid.NewString()
				if e := tx.Table("catalog_options").Create(map[string]any{"id": oid, "title": o.Title, "is_exclusive": true, "created_at": now, "updated_at": now}).Error; e != nil {
					return e
				}
			} else {
				oid = *o.ID
			}
			var link struct{ ID string }
			if newGroup || !current[oid].Selected {
				link.ID = "prodopt_" + uuid.NewString()
				if e := tx.Table("catalog_product_options").Create(map[string]any{"id": link.ID, "product_id": p, "product_option_id": oid, "created_at": now, "updated_at": now}).Error; e != nil {
					return e
				}
			} else {
				if e := tx.Table("catalog_product_options").Select("id").Where("product_id=? AND product_option_id=? AND deleted_at IS NULL", p, oid).Take(&link).Error; e != nil {
					return e
				}
			}
			keep := []string{}
			prior := map[string]bool{}
			maxRank := 0
			for _, v := range current[oid].Values {
				prior[v.ID] = v.Selected
				if v.Rank != nil && *v.Rank > maxRank {
					maxRank = *v.Rank
				}
			}
			for _, v := range o.Values {
				vid := ""
				if v.ID == nil {
					vid = "optval_" + uuid.NewString()
					maxRank++
					if e := tx.Table("catalog_option_values").Create(map[string]any{"id": vid, "option_id": oid, "value": v.Value, "rank": maxRank, "created_at": now, "updated_at": now}).Error; e != nil {
						return e
					}
				} else {
					vid = *v.ID
				}
				keep = append(keep, vid)
				if !prior[vid] {
					if e := tx.Table("catalog_product_option_values").Create(map[string]any{"id": "prodoptval_" + uuid.NewString(), "product_product_option_id": link.ID, "product_option_value_id": vid, "created_at": now, "updated_at": now}).Error; e != nil {
						return e
					}
				}
			}
			if e := tx.Table("catalog_product_option_values").Where("product_product_option_id=? AND deleted_at IS NULL AND product_option_value_id NOT IN ?", link.ID, keep).Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; e != nil {
				return e
			}
		}
		if e := tx.Table("catalog_products").Where("id=?", p).Update("updated_at", now).Error; e != nil {
			return e
		}
		after, e := loadProductOptions(tx, p, enabled)
		if e != nil {
			return e
		}
		oldJSON, _ := json.Marshal(before)
		newJSON, _ := json.Marshal(after)
		out = ImageResult{after.Revision, false}
		return tx.Table("catalog_option_edits").Create(map[string]any{"operation_id": in.OperationID, "product_id": p, "actor_id": actor.ID, "request_sha256": requestSHA, "before_json": string(oldJSON), "after_json": string(newJSON), "result_revision": after.Revision, "created_at": now}).Error
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}
