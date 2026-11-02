package catalog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"math"
	"sort"
	"time"
	"unicode/utf8"
)

var ErrTrackedInventory = errors.New("device inventory identity must remain unchanged")

type InventoryRequirement struct {
	InventoryItemID  string `json:"inventory_item_id"`
	RequiredQuantity string `json:"required_quantity"`
}
type VariantInventoryInput struct {
	OperationID      string                 `json:"operation_id"`
	ExpectedRevision string                 `json:"expected_revision"`
	Items            []InventoryRequirement `json:"items"`
}
type InventoryChoice struct {
	ID            string  `json:"id"`
	SKU           *string `json:"sku"`
	Title         *string `json:"title"`
	UnitOfMeasure *string `json:"unit_of_measure"`
	DeviceBound   bool    `json:"device_bound"`
}
type InventoryChoices struct {
	Items   []InventoryChoice `json:"items"`
	Offset  int               `json:"offset"`
	Limit   int               `json:"limit"`
	HasMore bool              `json:"has_more"`
}
type ManagedInventoryItem struct {
	InventoryRequirement
	LinkID        string    `json:"link_id"`
	SKU           *string   `json:"sku"`
	Title         *string   `json:"title"`
	UnitOfMeasure *string   `json:"unit_of_measure"`
	Available     bool      `json:"available"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type VariantInventoryContext struct {
	ProductID             string                 `json:"product_id"`
	VariantID             string                 `json:"variant_id"`
	Enabled               bool                   `json:"enabled"`
	Revision              string                 `json:"revision"`
	ManageInventory       bool                   `json:"manage_inventory"`
	DeviceInventoryItemID *string                `json:"device_inventory_item_id"`
	Items                 []ManagedInventoryItem `json:"items"`
}

func validInventoryLinks(in VariantInventoryInput) bool {
	if !validVariantImages(VariantImageInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, ImageIDs: []string{}}) || in.Items == nil || len(in.Items) > 1000 {
		return false
	}
	seen := map[string]bool{}
	for _, i := range in.Items {
		if !validProductID(i.InventoryItemID) || seen[i.InventoryItemID] {
			return false
		}
		seen[i.InventoryItemID] = true
		if _, e := inventory.RequiredInventoryQuantity("1", i.RequiredQuantity); e != nil {
			return false
		}
	}
	return true
}
func decodeInventoryLinks(r io.Reader) (VariantInventoryInput, error) {
	var in VariantInventoryInput
	raw, e := io.ReadAll(io.LimitReader(r, 262145))
	if e != nil {
		return in, e
	}
	if len(raw) > 262144 || !utf8.Valid(raw) {
		return in, errEditInput
	}
	fields, e := imageObject(raw, "operation_id", "expected_revision", "items")
	if e != nil {
		return in, e
	}
	for _, v := range fields {
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return in, errEditInput
		}
	}
	var items []json.RawMessage
	if json.Unmarshal(fields["items"], &items) != nil {
		return in, errEditInput
	}
	for _, i := range items {
		if _, e := imageObject(i, "inventory_item_id", "required_quantity"); e != nil {
			return in, e
		}
	}
	if json.Unmarshal(raw, &in) != nil || !validInventoryLinks(in) {
		return in, errEditInput
	}
	return in, nil
}
func inventoryLinkAccess(ctx context.Context, tx *gorm.DB, actor identity.User) error {
	if e := authorization.CheckTx(ctx, tx, actor, authorization.ProductRead, authorization.ProductWrite, authorization.InventoryRead); e != nil {
		return e
	}
	if e := Ready(ctx, tx); e != nil {
		return e
	}
	return inventory.Ready(ctx, tx)
}

// Synchronize relation changes with device enrollment before taking product
// locks. The gate can be disabled: this command does not mutate device facts.
func lockInventoryIdentity(tx *gorm.DB) error {
	var gate struct {
		Version  int
		Revision uint64
	}
	if tx.Table("device_runtime_schema").Where("id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&gate).Error != nil || gate.Version != 1 || gate.Revision == 0 || gate.Revision == math.MaxUint64 {
		return ErrSchema
	}
	return nil
}
func lockInventoryLinks(tx *gorm.DB, p, v string) (bool, error) {
	var cap struct {
		Version int
		Enabled bool
	}
	if tx.Table("catalog_command_capabilities").Where("command='variant.inventory'").Clauses(clause.Locking{Strength: "SHARE"}).Take(&cap).Error != nil || cap.Version != 1 {
		return false, ErrSchema
	}
	if e := lockInventoryIdentity(tx); e != nil {
		return false, e
	}
	var row struct{ ID string }
	if e := tx.Table("catalog_products").Select("id").Where("id=? AND deleted_at IS NULL", p).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error; e != nil {
		return false, e
	}
	if e := lockImageVariant(tx, p, v); e != nil {
		return false, e
	}
	return cap.Enabled, nil
}
func trackedInventoryItem(tx *gorm.DB, v string) (*string, error) {
	var ids []string
	if tx.Table("device_variant_tracking").Where("variant_id=? AND deleted_at IS NULL", v).Order("id").Limit(2).Pluck("inventory_item_id", &ids).Error != nil || len(ids) > 1 {
		return nil, ErrSchema
	}
	if len(ids) == 1 {
		return &ids[0], nil
	}
	return nil, nil
}
func validateTrackedInventory(tx *gorm.DB, v string, items []InventoryRequirement) error {
	tracked, e := trackedInventoryItem(tx, v)
	if e != nil {
		return e
	}
	if tracked != nil {
		if len(items) != 1 || items[0].InventoryItemID != *tracked {
			return ErrTrackedInventory
		}
		same, e := inventory.Equal(items[0].RequiredQuantity, "1")
		if e != nil || !same {
			return ErrTrackedInventory
		}
	}
	ids := []string{}
	for _, i := range items {
		ids = append(ids, i.InventoryItemID)
	}
	if len(ids) > 0 {
		var n int64
		if tx.Table("device_variant_tracking t").Where("t.inventory_item_id IN ? AND t.variant_id<>? AND t.deleted_at IS NULL", ids, v).Count(&n).Error != nil {
			return ErrSchema
		}
		if n > 0 {
			return ErrTrackedInventory
		}
	}
	return nil
}
func loadInventoryLinks(tx *gorm.DB, p, v string, enabled bool) (VariantInventoryContext, error) {
	out := VariantInventoryContext{ProductID: p, VariantID: v, Enabled: enabled, Items: []ManagedInventoryItem{}}
	var row struct {
		ManageInventory bool
		UpdatedAt       time.Time
	}
	if e := tx.Table("catalog_variants").Select("manage_inventory,updated_at").Where("id=? AND product_id=? AND deleted_at IS NULL", v, p).Take(&row).Error; e != nil {
		return out, e
	}
	out.ManageInventory = row.ManageInventory
	if tx.Table("catalog_variant_inventory_items l").Select("l.id AS link_id,l.inventory_item_id,CAST(l.required_quantity AS CHAR) AS required_quantity,l.updated_at,i.sku,i.title,i.unit_of_measure,(i.id IS NOT NULL) AS available").Joins("LEFT JOIN inventory_items i ON i.id=l.inventory_item_id AND i.deleted_at IS NULL").Where("l.variant_id=? AND l.deleted_at IS NULL", v).Order("l.inventory_item_id").Limit(1001).Scan(&out.Items).Error != nil || len(out.Items) > 1000 {
		return out, ErrSchema
	}
	var e error
	out.DeviceInventoryItemID, e = trackedInventoryItem(tx, v)
	if e != nil {
		return out, e
	}
	out.Revision = imageDigest(struct {
		ProductID, VariantID string
		Manage               bool
		UpdatedAt            time.Time
		Items                []ManagedInventoryItem
		Tracked              *string
	}{p, v, out.ManageInventory, row.UpdatedAt, out.Items, out.DeviceInventoryItemID})
	return out, nil
}
func (h Handler) ReadInventoryLinks(ctx context.Context, p, v string, actor identity.User) (VariantInventoryContext, error) {
	var out VariantInventoryContext
	if !validProductID(p) || !validProductID(v) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := inventoryLinkAccess(ctx, tx, actor); e != nil {
			return e
		}
		enabled, e := lockInventoryLinks(tx, p, v)
		if e != nil {
			return e
		}
		out, e = loadInventoryLinks(tx, p, v, enabled)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}
func (h Handler) InventoryLinkChoices(ctx context.Context, p, v, q string, offset int, actor identity.User) (InventoryChoices, error) {
	out := InventoryChoices{Items: []InventoryChoice{}, Offset: offset, Limit: 50}
	if !validProductID(p) || !validProductID(v) || !utf8.ValidString(q) || utf8.RuneCountInString(q) > 200 || containsControl(q) || offset < 0 || offset > 1000000 {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := inventoryLinkAccess(ctx, tx, actor); e != nil {
			return e
		}
		if _, e := lockInventoryLinks(tx, p, v); e != nil {
			return e
		}
		query := tx.Table("inventory_items i").Select("i.id,i.sku,i.title,i.unit_of_measure,EXISTS(SELECT 1 FROM device_variant_tracking t WHERE t.inventory_item_id=i.id AND t.variant_id<>? AND t.deleted_at IS NULL) AS device_bound", v).Where("i.deleted_at IS NULL")
		if q != "" {
			query = query.Where("LOCATE(LOWER(?),LOWER(COALESCE(i.title,'')))>0 OR LOCATE(LOWER(?),LOWER(COALESCE(i.sku,'')))>0 OR LOCATE(LOWER(?),LOWER(i.id))>0", q, q, q)
		}
		if e := query.Order("i.id").Offset(offset).Limit(51).Scan(&out.Items).Error; e != nil {
			return e
		}
		out.HasMore = len(out.Items) > 50
		if out.HasMore {
			out.Items = out.Items[:50]
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}

type retiredInventoryLink struct {
	ID, InventoryItemID, RequiredQuantity string
	CreatedAt, UpdatedAt                  time.Time
	DeletedAt                             *time.Time
}

func (h Handler) EditInventoryLinks(ctx context.Context, p, v string, actor identity.User, in VariantInventoryInput) (ImageResult, error) {
	var out ImageResult
	if !validProductID(p) || !validProductID(v) || !validInventoryLinks(in) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	requestSHA := imageDigest(struct {
		VariantID string
		Input     VariantInventoryInput
	}{v, in})
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := inventoryLinkAccess(ctx, tx, actor); e != nil {
			return e
		}
		enabled, e := lockInventoryLinks(tx, p, v)
		if e != nil {
			return e
		}
		var receipt struct{ ProductID, ActorID, RequestSHA256, ResultRevision string }
		e = tx.Table("catalog_inventory_edits").Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&receipt).Error
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
		before, e := loadInventoryLinks(tx, p, v, enabled)
		if e != nil {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		if e := validateTrackedInventory(tx, v, in.Items); e != nil {
			return e
		}
		wanted := map[string]string{}
		ids := []string{}
		for _, i := range in.Items {
			q, e := inventory.RequiredInventoryQuantity("1", i.RequiredQuantity)
			if e != nil {
				return errEditInput
			}
			wanted[i.InventoryItemID] = q
			ids = append(ids, i.InventoryItemID)
		}
		sort.Strings(ids)
		for _, id := range ids {
			var item struct{ ID string }
			if e := tx.Table("inventory_items").Select("id").Where("id=? AND deleted_at IS NULL", id).Clauses(clause.Locking{Strength: "SHARE"}).Take(&item).Error; e != nil {
				if errors.Is(e, gorm.ErrRecordNotFound) {
					return errEditInput
				}
				return e
			}
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		prior := map[string]ManagedInventoryItem{}
		for _, old := range before.Items {
			prior[old.InventoryItemID] = old
			if _, ok := wanted[old.InventoryItemID]; !ok {
				if e := tx.Table("catalog_variant_inventory_items").Where("variant_id=? AND inventory_item_id=? AND deleted_at IS NULL", v, old.InventoryItemID).Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; e != nil {
					return e
				}
			}
		}
		restored := []retiredInventoryLink{}
		for _, id := range ids {
			if old, ok := prior[id]; ok {
				same, _ := inventory.Equal(old.RequiredQuantity, wanted[id])
				if !same {
					if e := tx.Table("catalog_variant_inventory_items").Where("variant_id=? AND inventory_item_id=? AND deleted_at IS NULL", v, id).Updates(map[string]any{"required_quantity": wanted[id], "updated_at": now}).Error; e != nil {
						return e
					}
				}
				continue
			}
			var old retiredInventoryLink
			e := tx.Table("catalog_variant_inventory_items").Select("id,inventory_item_id,CAST(required_quantity AS CHAR) AS required_quantity,created_at,updated_at,deleted_at").Where("variant_id=? AND inventory_item_id=?", v, id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&old).Error
			if e == nil {
				if old.DeletedAt == nil {
					return ErrSchema
				}
				restored = append(restored, old)
				if e := tx.Table("catalog_variant_inventory_items").Where("variant_id=? AND inventory_item_id=?", v, id).Updates(map[string]any{"deleted_at": nil, "required_quantity": wanted[id], "updated_at": now}).Error; e != nil {
					return e
				}
			} else if errors.Is(e, gorm.ErrRecordNotFound) {
				if e := tx.Table("catalog_variant_inventory_items").Create(map[string]any{"id": "pvii_" + uuid.NewString(), "variant_id": v, "inventory_item_id": id, "required_quantity": wanted[id], "created_at": now, "updated_at": now}).Error; e != nil {
					return e
				}
			} else {
				return e
			}
		}
		if e := tx.Table("catalog_variants").Where("id=?", v).Update("updated_at", now).Error; e != nil {
			return e
		}
		after, e := loadInventoryLinks(tx, p, v, enabled)
		if e != nil {
			return e
		}
		oldJSON, _ := json.Marshal(struct {
			Context  VariantInventoryContext
			Restored []retiredInventoryLink
		}{before, restored})
		newJSON, _ := json.Marshal(after)
		out = ImageResult{after.Revision, false}
		return tx.Table("catalog_inventory_edits").Create(map[string]any{"operation_id": in.OperationID, "product_id": p, "actor_id": actor.ID, "request_sha256": requestSHA, "before_json": string(oldJSON), "after_json": string(newJSON), "result_revision": after.Revision, "created_at": now}).Error
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}
