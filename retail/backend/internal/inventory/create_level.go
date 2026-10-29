package inventory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CreateLevelInput struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	LocationID       string `json:"location_id"`
	StockedQuantity  string `json:"stocked_quantity"`
	IncomingQuantity string `json:"incoming_quantity"`
}
type CreateLevelLocation struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	HasLevel  bool      `json:"has_level" gorm:"-"`
	UpdatedAt time.Time `json:"-"`
}
type CreateLevelContext struct {
	ProductID       string                `json:"product_id"`
	VariantID       string                `json:"variant_id"`
	InventoryItemID string                `json:"inventory_item_id"`
	Locations       []CreateLevelLocation `json:"locations"`
	SharedVariants  int                   `json:"shared_variants"`
	Enabled         bool                  `json:"enabled"`
	Revision        string                `json:"revision"`
}
type CreateLevelResult struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}

func validCreateLevel(in CreateLevelInput) bool {
	return validID(in.LocationID) && validStockInput(StockInput{in.OperationID, in.ExpectedRevision, in.StockedQuantity, in.IncomingQuantity})
}
func decodeCreateLevel(r io.Reader) (CreateLevelInput, error) {
	var in CreateLevelInput
	b, e := io.ReadAll(io.LimitReader(r, 4097))
	if e != nil || len(b) > 4096 || !utf8.Valid(b) {
		return in, errStockInput
	}
	d := json.NewDecoder(bytes.NewReader(b))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return in, errStockInput
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		tok, e = d.Token()
		k, ok := tok.(string)
		if e != nil || !ok || fields[k] != nil {
			return in, errStockInput
		}
		switch k {
		case "operation_id", "expected_revision", "location_id", "stocked_quantity", "incoming_quantity":
		default:
			return in, errStockInput
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return in, errStockInput
		}
		if _, e = importvalue.Text(raw, 191); e != nil {
			return in, errStockInput
		}
		fields[k] = raw
	}
	if tok, e = d.Token(); e != nil || tok != json.Delim('}') || len(fields) != 5 || d.Decode(new(any)) != io.EOF {
		return in, errStockInput
	}
	b, _ = json.Marshal(fields)
	if json.Unmarshal(b, &in) != nil || !validCreateLevel(in) {
		return in, errStockInput
	}
	return in, nil
}

// The command capability row is also the global creation lock. Every future
// level creator must share it; imports remain an offline, separately gated step.
func createLevelGate(ctx context.Context, tx *gorm.DB, actor identity.User) (bool, error) {
	if e := authorization.CheckTx(ctx, tx, actor, authorization.ProductRead, authorization.InventoryRead, authorization.StockWrite, authorization.WarehouseRead); e != nil {
		return false, e
	}
	var cap struct {
		Version int
		Enabled bool
	}
	if tx.Table("inventory_command_capabilities").Clauses(clause.Locking{Strength: "UPDATE"}).Where("command='inventory.create_level'").Take(&cap).Error != nil || cap.Version != 1 {
		return false, ErrSchema
	}
	var marker struct{ ID int }
	if tx.Table("inventory_runtime_schema").Select("id").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&marker).Error != nil || Ready(ctx, tx) != nil {
		return false, ErrSchema
	}
	return cap.Enabled, nil
}
func loadCreateLevel(tx *gorm.DB, p, v, item string, enabled bool) (CreateLevelContext, error) {
	out := CreateLevelContext{ProductID: p, VariantID: v, InventoryItemID: item, Enabled: enabled, Locations: []CreateLevelLocation{}}
	var link struct {
		ID, RequiredQuantity string
		UpdatedAt            time.Time
	}
	if e := tx.Table("catalog_variant_inventory_items vi").Select("vi.id,vi.required_quantity,vi.updated_at").Joins("JOIN catalog_variants v ON v.id=vi.variant_id AND v.deleted_at IS NULL").Joins("JOIN catalog_products p ON p.id=v.product_id AND p.deleted_at IS NULL").Where("vi.variant_id=? AND p.id=? AND vi.inventory_item_id=? AND vi.deleted_at IS NULL", v, p, item).Clauses(clause.Locking{Strength: "SHARE"}).Take(&link).Error; e != nil {
		return out, e
	}
	var inventory struct {
		ID         string
		SKU, Title *string
		UpdatedAt  time.Time
	}
	if e := tx.Table("inventory_items").Select("id,sku,title,updated_at").Where("id=? AND deleted_at IS NULL", item).Clauses(clause.Locking{Strength: "SHARE"}).Take(&inventory).Error; e != nil {
		return out, e
	}
	if tx.Table("inventory_locations").Select("id,name,updated_at").Where("deleted_at IS NULL").Order("id").Limit(1001).Scan(&out.Locations).Error != nil || len(out.Locations) > 1000 {
		return out, errLimit
	}
	var levels []stockState
	if tx.Table("inventory_levels").Select("id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,updated_at").Where("inventory_item_id=? AND deleted_at IS NULL", item).Order("id").Limit(10001).Scan(&levels).Error != nil || len(levels) > 10000 {
		return out, errLimit
	}
	assigned := map[string]bool{}
	for _, l := range levels {
		assigned[l.LocationID] = true
	}
	for i := range out.Locations {
		out.Locations[i].HasLevel = assigned[out.Locations[i].ID]
	}
	var holds []struct{ LocationID, Reason string }
	if tx.Table("inventory_migration_holds").Select("location_id,reason").Where("inventory_item_id=?", item).Order("location_id,reason").Limit(10001).Scan(&holds).Error != nil || len(holds) > 10000 {
		return out, errLimit
	}
	var reservations []struct {
		ID, LocationID, Quantity string
		UpdatedAt                time.Time
	}
	if tx.Table("inventory_reservations").Select("id,location_id,quantity,updated_at").Where("inventory_item_id=? AND deleted_at IS NULL", item).Order("id").Limit(10001).Scan(&reservations).Error != nil || len(reservations) > 10000 {
		return out, errLimit
	}
	var members []struct{ ID, VariantID string }
	if tx.Table("catalog_variant_inventory_items vi").Select("vi.id,vi.variant_id").Joins("JOIN catalog_variants v ON v.id=vi.variant_id AND v.deleted_at IS NULL").Joins("JOIN catalog_products p ON p.id=v.product_id AND p.deleted_at IS NULL").Where("vi.inventory_item_id=? AND vi.deleted_at IS NULL", item).Order("vi.id").Limit(10001).Scan(&members).Error != nil || len(members) > 10000 {
		return out, errLimit
	}
	seen := map[string]bool{}
	for _, m := range members {
		seen[m.VariantID] = true
	}
	out.SharedVariants = len(seen)
	// UpdatedAt is hidden in the transport DTO but remains a revision fact.
	dates := make([]time.Time, len(out.Locations))
	for i, l := range out.Locations {
		dates[i] = l.UpdatedAt
	}
	b, _ := json.Marshal([]any{p, v, item, link, inventory, out.Locations, dates, levels, holds, reservations, members})
	h := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(h[:])
	return out, nil
}
func (h Handler) ReadCreateLevel(ctx context.Context, actor identity.User, p, v, item string) (CreateLevelContext, error) {
	var out CreateLevelContext
	if !validID(p) || !validID(v) || !validID(item) {
		return out, errStockInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		enabled, e := createLevelGate(ctx, tx, actor)
		if e != nil {
			return e
		}
		out, e = loadCreateLevel(tx, p, v, item, enabled)
		return e
	})
	return out, e
}
func (h Handler) CreateLevel(ctx context.Context, actor identity.User, p, v, item string, in CreateLevelInput) (CreateLevelResult, error) {
	var out CreateLevelResult
	if !validID(p) || !validID(v) || !validID(item) || !validCreateLevel(in) {
		return out, errStockInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	request, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		enabled, e := createLevelGate(ctx, tx, actor)
		if e != nil {
			return e
		}
		if !enabled {
			return ErrSchema
		}
		before, e := loadCreateLevel(tx, p, v, item, enabled)
		if e != nil {
			return e
		}
		var location struct{ ID string }
		if e = tx.Table("inventory_locations").Select("id").Where("id=? AND deleted_at IS NULL", in.LocationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&location).Error; e != nil {
			return e
		}
		var receipt struct{ ActorID, ProductID, VariantID, InventoryItemID, LocationID, LevelID, RequestJSON, ResultRevision string }
		e = tx.Table("inventory_level_creations").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.ProductID != p || receipt.VariantID != v || receipt.InventoryItemID != item || receipt.LocationID != in.LocationID || receipt.RequestJSON != string(request) {
				return ErrStockConflict
			}
			var l struct{ ID string }
			if e = tx.Table("inventory_levels").Select("id").Where("id=? AND inventory_item_id=? AND location_id=? AND deleted_at IS NULL", receipt.LevelID, item, in.LocationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&l).Error; e != nil {
				return e
			}
			out = CreateLevelResult{receipt.LevelID, receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrStockConflict
		}
		var levels []struct{ ID string }
		if tx.Table("inventory_levels").Select("id").Where("inventory_item_id=? AND location_id=? AND deleted_at IS NULL", item, in.LocationID).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1).Find(&levels).Error != nil {
			return ErrSchema
		}
		if len(levels) > 0 {
			return ErrStockConflict
		}
		// Never hide a reservation or historical migration discrepancy by creating
		// an apparently empty, reconciled level alongside it.
		var holds []struct{ Reason string }
		if tx.Table("inventory_migration_holds").Select("reason").Where("inventory_item_id=? AND location_id=?", item, in.LocationID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Find(&holds).Error != nil {
			return ErrSchema
		}
		if len(holds) > 0 {
			return ErrStockHeld
		}
		var reservations []struct{ ID string }
		if tx.Table("inventory_reservations").Select("id").Where("inventory_item_id=? AND location_id=? AND deleted_at IS NULL", item, in.LocationID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Find(&reservations).Error != nil {
			return ErrSchema
		}
		if len(reservations) > 0 {
			return ErrStockHeld
		}
		raw := func(s string) string {
			b, _ := json.Marshal(map[string]any{"value": s, "precision": 38})
			return string(b)
		}
		zero := "0.000000000000000000"
		now := time.Now().UTC().Truncate(time.Microsecond)
		id := "ilevel_" + uuid.NewString()
		if tx.Table("inventory_levels").Create(map[string]any{"id": id, "inventory_item_id": item, "location_id": in.LocationID, "stocked_quantity": in.StockedQuantity, "incoming_quantity": in.IncomingQuantity, "reserved_quantity": zero, "raw_stocked_quantity": raw(in.StockedQuantity), "raw_incoming_quantity": raw(in.IncomingQuantity), "raw_reserved_quantity": raw(zero), "created_at": now, "updated_at": now}).Error != nil {
			return ErrSchema
		}
		after := stockState{ID: id, InventoryItemID: item, LocationID: in.LocationID, StockedQuantity: in.StockedQuantity, IncomingQuantity: in.IncomingQuantity, ReservedQuantity: zero, UpdatedAt: now}
		out = CreateLevelResult{id, stockRevision(after), false}
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(after)
		return tx.Table("inventory_level_creations").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "product_id": p, "variant_id": v, "inventory_item_id": item, "location_id": in.LocationID, "level_id": id, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_revision": out.Revision, "created_at": now}).Error
	})
	return out, e
}
func (h Handler) CreateLevelHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	fail := func(e error) {
		switch {
		case errors.Is(e, errStockInput):
			platform.Fail(c, 400, "inventory.invalid_edit", "Invalid stock level")
		case errors.Is(e, authorization.ErrDenied):
			platform.Fail(c, 403, "authorization.denied", "Current inventory and warehouse permission required")
		case errors.Is(e, gorm.ErrRecordNotFound):
			platform.Fail(c, 404, "inventory.not_found", "Inventory item, SKU or warehouse not found")
		case errors.Is(e, ErrStockHeld):
			platform.Fail(c, 409, "inventory.reconciliation_required", "Existing stock history requires reconciliation")
		case errors.Is(e, ErrStockConflict):
			platform.Fail(c, 409, "inventory.edit_conflict", "Warehouse stock or operation changed; reload")
		default:
			platform.Fail(c, 503, "inventory.unavailable", "Stock level creation unavailable; retry the same operation")
		}
	}
	if c.Request.URL.RawQuery != "" {
		fail(errStockInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	p, v, item := c.Param("id"), c.Param("variant_id"), c.Param("item_id")
	if c.Request.Method == "GET" {
		out, e := h.ReadCreateLevel(ctx, actor, p, v, item)
		if e != nil {
			fail(e)
			return
		}
		c.JSON(200, out)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "inventory.content_type", "JSON required")
		return
	}
	in, e := decodeCreateLevel(c.Request.Body)
	if e != nil {
		fail(e)
		return
	}
	out, e := h.CreateLevel(ctx, actor, p, v, item, in)
	if e != nil {
		fail(e)
		return
	}
	c.JSON(200, out)
}
