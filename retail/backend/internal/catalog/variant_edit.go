package catalog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/identity"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type VariantFields struct {
	Title           string   `json:"title"`
	SKU             *string  `json:"sku"`
	Barcode         *string  `json:"barcode"`
	EAN             *string  `json:"ean"`
	UPC             *string  `json:"upc"`
	Material        *string  `json:"material"`
	HSCode          *string  `json:"hs_code"`
	MidCode         *string  `json:"mid_code"`
	OriginCountry   *string  `json:"origin_country"`
	Weight          *float64 `json:"weight"`
	Length          *float64 `json:"length"`
	Height          *float64 `json:"height"`
	Width           *float64 `json:"width"`
	ManageInventory bool     `json:"manage_inventory"`
	AllowBackorder  bool     `json:"allow_backorder"`
}
type VariantEditInput struct {
	VariantFields
	OperationID      string   `json:"operation_id"`
	ExpectedRevision string   `json:"expected_revision"`
	OptionValueIDs   []string `json:"option_value_ids"`
}
type VariantEditContext struct {
	ProductID      string        `json:"product_id"`
	VariantID      string        `json:"variant_id"`
	Enabled        bool          `json:"enabled"`
	Revision       string        `json:"revision"`
	Variant        VariantFields `json:"variant"`
	Options        []AdminOption `json:"options"`
	SelectedIDs    []string      `json:"selected_ids"`
	InventoryLinks int           `json:"inventory_links"`
}

var ErrVariantCodeConflict = errors.New("variant code already used")
var ErrInventoryCodeConflict = errors.New("default inventory SKU already used")

func validVariantEdit(in VariantEditInput) bool {
	if !validVariantImages(VariantImageInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, ImageIDs: []string{}}) || !createText(in.Title, 500) || in.OptionValueIDs == nil || len(in.OptionValueIDs) > 1000 {
		return false
	}
	for _, v := range []*string{in.SKU, in.Barcode, in.EAN, in.UPC, in.Material, in.HSCode, in.MidCode, in.OriginCountry} {
		if v != nil && (!utf8.ValidString(*v) || utf8.RuneCountInString(*v) > 512 || containsControl(*v)) {
			return false
		}
	}
	for _, v := range []*float64{in.Weight, in.Length, in.Height, in.Width} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0)) {
			return false
		}
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
func decodeVariantEdit(r io.Reader) (VariantEditInput, error) {
	var in VariantEditInput
	raw, e := io.ReadAll(io.LimitReader(r, 262145))
	if e != nil {
		return in, e
	}
	if len(raw) > 262144 || !utf8.Valid(raw) {
		return in, errEditInput
	}
	fields, e := imageObject(raw, "operation_id", "expected_revision", "title", "sku", "barcode", "ean", "upc", "material", "hs_code", "mid_code", "origin_country", "weight", "length", "height", "width", "manage_inventory", "allow_backorder", "option_value_ids")
	if e != nil {
		return in, e
	}
	for _, key := range []string{"operation_id", "expected_revision", "title", "manage_inventory", "allow_backorder", "option_value_ids"} {
		if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return in, errEditInput
		}
	}
	if json.Unmarshal(raw, &in) != nil || !validVariantEdit(in) {
		return in, errEditInput
	}
	return in, nil
}

// SKU creation and editing share the global numbering lock before product locks.
// Images and option editing also lock the product, so no SKU/configuration race
// can bypass the complete-set check or the color image synchronization.
func lockVariantEdit(tx *gorm.DB, p, v string) (bool, variantCreateRuntime, error) {
	var cap struct {
		Version int
		Enabled bool
	}
	if tx.Table("catalog_command_capabilities").Where("command='variant.edit'").Clauses(clause.Locking{Strength: "SHARE"}).Take(&cap).Error != nil || cap.Version != 1 {
		return false, variantCreateRuntime{}, ErrSchema
	}
	rt, e := lockVariantCreate(tx)
	if e != nil {
		return false, rt, e
	}
	if e := lockInventoryIdentity(tx); e != nil {
		return false, rt, e
	}
	var product struct{ ID string }
	if e := tx.Table("catalog_products").Select("id").Where("id=? AND deleted_at IS NULL", p).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&product).Error; e != nil {
		return false, rt, e
	}
	if e := lockImageVariant(tx, p, v); e != nil {
		return false, rt, e
	}
	return cap.Enabled, rt, nil
}

type editInventoryLink struct {
	ID, InventoryItemID, RequiredQuantity string
	UpdatedAt                             time.Time
}

func loadVariantEdit(tx *gorm.DB, p, v string, enabled bool) (VariantEditContext, error) {
	out := VariantEditContext{ProductID: p, VariantID: v, Enabled: enabled, SelectedIDs: []string{}}
	if e := tx.Table("catalog_variants").Where("id=? AND product_id=? AND deleted_at IS NULL", v, p).Take(&out.Variant).Error; e != nil {
		return out, e
	}
	var e error
	out.Options, e = productOptions(tx, p)
	if e != nil {
		return out, e
	}
	if tx.Table("catalog_variant_options").Where("variant_id=?", v).Order("option_value_id").Pluck("option_value_id", &out.SelectedIDs).Error != nil {
		return out, ErrSchema
	}
	var links []editInventoryLink
	if tx.Table("catalog_variant_inventory_items").Select("id,inventory_item_id,CAST(required_quantity AS CHAR) AS required_quantity,updated_at").Where("variant_id=? AND deleted_at IS NULL", v).Order("inventory_item_id").Limit(1001).Clauses(clause.Locking{Strength: "SHARE"}).Find(&links).Error != nil || len(links) > 1000 {
		return out, ErrSchema
	}
	out.InventoryLinks = len(links)
	images, e := loadVariantImages(tx, p, v, false)
	if e != nil {
		return out, e
	}
	var product struct{ Title, Handle string }
	if tx.Table("catalog_products").Select("title,handle").Where("id=?", p).Take(&product).Error != nil {
		return out, ErrSchema
	}
	var stamp struct{ UpdatedAt time.Time }
	if tx.Table("catalog_variants").Select("updated_at").Where("id=?", v).Take(&stamp).Error != nil {
		return out, ErrSchema
	}
	out.Revision = imageDigest(struct {
		Variant   VariantFields
		Options   []AdminOption
		Selected  []string
		Links     []editInventoryLink
		Images    VariantImageContext
		Product   struct{ Title, Handle string }
		UpdatedAt time.Time
	}{out.Variant, out.Options, out.SelectedIDs, links, images, product, stamp.UpdatedAt})
	return out, nil
}
func (h Handler) ReadVariantEdit(ctx context.Context, p, v string, actor identity.User) (VariantEditContext, error) {
	var out VariantEditContext
	if !validProductID(p) || !validProductID(v) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := imageAccess(ctx, tx, actor); e != nil {
			return e
		}
		enabled, _, e := lockVariantEdit(tx, p, v)
		if e != nil {
			return e
		}
		out, e = loadVariantEdit(tx, p, v, enabled)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}
func nullableVariantText(s *string) *string {
	if s != nil && *s == "" {
		return nil
	}
	return s
}

func validateEditedOptions(tx *gorm.DB, p, v string, options []AdminOption, ids []string) error {
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	if len(ids) != len(options) {
		return errEditInput
	}
	for _, o := range options {
		n := 0
		for _, value := range o.Values {
			if selected[value.ID] {
				n++
			}
		}
		if n != 1 {
			return errEditInput
		}
	}
	var otherIDs []string
	if tx.Table("catalog_variants").Where("product_id=? AND id<>? AND deleted_at IS NULL", p, v).Order("id").Limit(10001).Pluck("id", &otherIDs).Error != nil || len(otherIDs) > 10000 {
		return ErrSchema
	}
	var values []creationOption
	if len(otherIDs) > 0 {
		if tx.Table("catalog_variant_options").Where("variant_id IN ?", otherIDs).Order("variant_id,option_value_id").Limit(100001).Find(&values).Error != nil || len(values) > 100000 {
			return ErrSchema
		}
	}
	sets := map[string][]string{}
	for _, id := range otherIDs {
		sets[id] = []string{}
	}
	for _, x := range values {
		sets[x.VariantID] = append(sets[x.VariantID], x.OptionValueID)
	}
	if variantOptionCollision(ids, sets) {
		return ErrEditConflict
	}
	return nil
}

// Source checkIfVariantWithOptionsAlreadyExists skips zero-dimensional SKUs.
// For a configured SKU, every selected value must not already occur together.
func variantOptionCollision(ids []string, sets map[string][]string) bool {
	if len(ids) == 0 {
		return false
	}
	for _, set := range sets {
		values := map[string]bool{}
		for _, v := range set {
			values[v] = true
		}
		same := true
		for _, id := range ids {
			if !values[id] {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	return false
}
func resolveEditedSKU(tx *gorm.DB, old *string, requested *string, rt *variantCreateRuntime) (string, error) {
	sku := ""
	if requested != nil {
		sku = *requested
	}
	if strings.TrimSpace(sku) == "" && old != nil && reservedSKUNumber(*old) > 0 {
		return *old, nil
	}
	if strings.TrimSpace(sku) == "" {
		var max struct{ N int64 }
		if tx.Raw("SELECT COALESCE(MAX(CAST(SUBSTRING(sku,4) AS SIGNED)-10000),0) AS n FROM (SELECT sku FROM catalog_variants UNION ALL SELECT sku FROM inventory_items) codes WHERE sku REGEXP '^ERP[0-9]{5,18}$'").Scan(&max).Error != nil {
			return "", ErrSchema
		}
		if max.N > rt.LastSkuNumber {
			rt.LastSkuNumber = max.N
		}
		if rt.LastSkuNumber >= math.MaxInt64-10000 {
			return "", ErrSchema
		}
		rt.LastSkuNumber++
		sku = "ERP" + strconv.FormatInt(rt.LastSkuNumber+10000, 10)
	}
	if n := reservedSKUNumber(sku); n > rt.LastSkuNumber {
		rt.LastSkuNumber = n
	}
	return sku, nil
}

// Match core update workflow: disabling dismisses links but never deletes stock,
// reservations or inventory records. Enabling with no active link creates one
// default item. A conflicting inventory SKU aborts rather than taking its stock.
func updateEditedInventory(tx *gorm.DB, v string, in VariantFields, now time.Time) error {
	if !in.ManageInventory {
		return tx.Table("catalog_variant_inventory_items").Where("variant_id=? AND deleted_at IS NULL", v).Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error
	}
	var n int64
	if e := tx.Table("catalog_variant_inventory_items").Where("variant_id=? AND deleted_at IS NULL", v).Count(&n).Error; e != nil {
		return e
	}
	if n > 0 {
		return nil
	}
	if in.SKU != nil {
		if e := tx.Table("inventory_items").Where("sku=? AND deleted_at IS NULL", *in.SKU).Count(&n).Error; e != nil {
			return e
		}
		if n > 0 {
			return ErrInventoryCodeConflict
		}
	}
	item := "iitem_" + uuid.NewString()
	if e := tx.Table("inventory_items").Create(map[string]any{"id": item, "sku": in.SKU, "title": in.Title, "description": in.Title, "origin_country": in.OriginCountry, "hs_code": in.HSCode, "mid_code": in.MidCode, "material": in.Material, "weight": in.Weight, "length": in.Length, "height": in.Height, "width": in.Width, "requires_shipping": true, "created_at": now, "updated_at": now}).Error; e != nil {
		return e
	}
	return tx.Table("catalog_variant_inventory_items").Create(map[string]any{"id": "pvii_" + uuid.NewString(), "variant_id": v, "inventory_item_id": item, "required_quantity": 1, "created_at": now, "updated_at": now}).Error
}
func (h Handler) EditVariant(ctx context.Context, p, v string, actor identity.User, in VariantEditInput) (ImageResult, error) {
	var out ImageResult
	if !validProductID(p) || !validProductID(v) || !validVariantEdit(in) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	requestSHA := imageDigest(struct {
		VariantID string
		Input     VariantEditInput
	}{v, in})
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := imageAccess(ctx, tx, actor); e != nil {
			return e
		}
		enabled, rt, e := lockVariantEdit(tx, p, v)
		if e != nil {
			return e
		}
		var receipt struct{ ProductID, ActorID, RequestSHA256, ResultRevision string }
		e = tx.Table("catalog_variant_edits").Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&receipt).Error
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
		before, e := loadVariantEdit(tx, p, v, enabled)
		if e != nil {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		if e := validateEditedOptions(tx, p, v, before.Options, in.OptionValueIDs); e != nil {
			return e
		}
		sku, e := resolveEditedSKU(tx, before.Variant.SKU, in.SKU, &rt)
		if e != nil {
			return e
		}
		if !in.ManageInventory {
			tracked, e := trackedInventoryItem(tx, v)
			if e != nil {
				return e
			}
			if tracked != nil {
				return ErrTrackedInventory
			}
		}
		fields := in.VariantFields
		fields.SKU = &sku
		fields.Barcode = nullableVariantText(fields.Barcode)
		fields.EAN = nullableVariantText(fields.EAN)
		fields.UPC = nullableVariantText(fields.UPC)
		fields.Material = nullableVariantText(fields.Material)
		fields.HSCode = nullableVariantText(fields.HSCode)
		fields.MidCode = nullableVariantText(fields.MidCode)
		fields.OriginCountry = nullableVariantText(fields.OriginCountry)
		for key, value := range map[string]*string{"sku": fields.SKU, "barcode": fields.Barcode, "ean": fields.EAN, "upc": fields.UPC} {
			if value != nil {
				var n int64
				if e := tx.Table("catalog_variants").Where(key+"=? AND id<>? AND deleted_at IS NULL", *value, v).Count(&n).Error; e != nil {
					return e
				}
				if n > 0 {
					return ErrVariantCodeConflict
				}
			}
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		raw, _ := json.Marshal(fields)
		updates := map[string]any{}
		if json.Unmarshal(raw, &updates) != nil {
			return ErrSchema
		}
		updates["updated_at"] = now
		if e := tx.Table("catalog_variants").Where("id=?", v).Updates(updates).Error; e != nil {
			return e
		}
		selected := map[string]bool{}
		for _, id := range in.OptionValueIDs {
			selected[id] = true
		}
		// The source association has a composite primary key and no deletion columns.
		if e := tx.Table("catalog_variant_options").Where("variant_id=?", v).Delete(map[string]any{}).Error; e != nil {
			return e
		}
		ids := append([]string{}, in.OptionValueIDs...)
		sort.Strings(ids)
		for _, id := range ids {
			if e := tx.Table("catalog_variant_options").Create(map[string]any{"variant_id": v, "option_value_id": id}).Error; e != nil {
				return e
			}
		}
		var product struct{ Title, Handle string }
		if tx.Table("catalog_products").Select("title,handle").Where("id=?", p).Take(&product).Error != nil {
			return ErrSchema
		}
		if e := syncEditedVariantImages(tx, p, v, iphoneVariantImageKey(product.Handle, product.Title, before.Options, selected), now); e != nil {
			return e
		}
		if e := updateEditedInventory(tx, v, fields, now); e != nil {
			return e
		}
		if e := tx.Exec("UPDATE catalog_variant_create_runtime SET revision=revision+1,last_sku_number=? WHERE id=1", rt.LastSkuNumber).Error; e != nil {
			return e
		}
		after, e := loadVariantEdit(tx, p, v, enabled)
		if e != nil {
			return e
		}
		oldJSON, _ := json.Marshal(before)
		newJSON, _ := json.Marshal(after)
		out = ImageResult{after.Revision, false}
		return tx.Table("catalog_variant_edits").Create(map[string]any{"operation_id": in.OperationID, "product_id": p, "actor_id": actor.ID, "request_sha256": requestSHA, "before_json": string(oldJSON), "after_json": string(newJSON), "result_revision": after.Revision, "created_at": now}).Error
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	var duplicate *driver.MySQLError
	if errors.As(e, &duplicate) && duplicate.Number == 1062 {
		e = ErrVariantCodeConflict
	}
	return out, e
}

func syncEditedVariantImages(tx *gorm.DB, p, v, key string, now time.Time) error {
	if key == "" {
		return nil
	}
	var gallery []AdminImage
	if tx.Table("catalog_images").Select("id,url,`rank`").Where("product_id=? AND deleted_at IS NULL", p).Order("url,id").Limit(1001).Scan(&gallery).Error != nil || len(gallery) > 1000 {
		return ErrSchema
	}
	matches := []AdminImage{}
	selected := map[string]bool{}
	for _, i := range gallery {
		parts := iphoneImagePattern.FindStringSubmatch(i.URL)
		if len(parts) == 3 && parts[1]+"/"+imageColor(parts[2]) == key {
			matches = append(matches, i)
			selected[i.ID] = true
		}
	}
	// Original subscriber preserves manual images when the new color has no match.
	if len(matches) == 0 {
		return nil
	}
	var links []struct{ ID, ImageID string }
	if e := tx.Table("catalog_variant_images").Select("id,image_id").Where("variant_id=? AND deleted_at IS NULL", v).Find(&links).Error; e != nil {
		return e
	}
	prior := map[string]bool{}
	for _, l := range links {
		prior[l.ImageID] = true
		if !selected[l.ImageID] {
			if e := tx.Table("catalog_variant_images").Where("id=?", l.ID).Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; e != nil {
				return e
			}
		}
	}
	for _, i := range matches {
		if !prior[i.ID] {
			if e := tx.Table("catalog_variant_images").Create(map[string]any{"id": "vimg_" + uuid.NewString(), "variant_id": v, "image_id": i.ID, "created_at": now, "updated_at": now}).Error; e != nil {
				return e
			}
		}
	}
	return tx.Table("catalog_variants").Where("id=?", v).Update("thumbnail", matches[0].URL).Error
}
