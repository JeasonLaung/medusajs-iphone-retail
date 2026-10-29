package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInput = errors.New("invalid device command")
var ErrConflict = errors.New("device configuration changed")
var ErrNotFound = errors.New("device SKU unavailable")

type Handler struct {
	DB             *gorm.DB
	WarehouseScope *WarehouseIntakeScope
}
type Tracking struct {
	ProductID       string `json:"product_id"`
	VariantID       string `json:"variant_id"`
	Revision        string `json:"revision"`
	Enabled         bool   `json:"enabled"`
	Eligible        bool   `json:"eligible"`
	Reason          string `json:"reason"`
	InventoryItemID string `json:"inventory_item_id"`
}
type EnrollInput struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
}
type EnrollResult struct {
	Tracking Tracking `json:"tracking"`
	Replayed bool     `json:"replayed"`
}

func trackingAuthority(ctx context.Context, tx *gorm.DB, actor identity.User, write bool) error {
	actions := []authorization.Action{authorization.ProductRead, authorization.InventoryRead, authorization.DeviceRead}
	if write {
		actions = append(actions, authorization.DeviceEnroll)
	}
	return authorization.CheckTx(ctx, tx, actor, actions...)
}

// The caller holds current authority and the global device gate before this
// current locking read. Quantity stock and reservations are never written here.
func inspectTracking(tx *gorm.DB, productID, variantID string) (Tracking, error) {
	out := Tracking{ProductID: productID, VariantID: variantID}
	var product struct{ ID string }
	if e := tx.Table("catalog_products").Select("id").Where("id=? AND deleted_at IS NULL", productID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&product).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return out, ErrNotFound
		}
		return out, ErrUnavailable
	}
	var v struct {
		ID                              string
		ManageInventory, AllowBackorder bool
	}
	if e := tx.Table("catalog_variants").Select("id,manage_inventory,allow_backorder").Where("id=? AND product_id=? AND deleted_at IS NULL", variantID, productID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&v).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return out, ErrNotFound
		}
		return out, ErrUnavailable
	}
	var links []struct{ ID, InventoryItemID, RequiredQuantity string }
	if tx.Table("catalog_variant_inventory_items").Select("id,inventory_item_id,CAST(required_quantity AS CHAR) AS required_quantity").Where("variant_id=? AND deleted_at IS NULL", variantID).Order("id").Limit(1001).Clauses(clause.Locking{Strength: "SHARE"}).Find(&links).Error != nil || len(links) > 1000 {
		return out, ErrUnavailable
	}
	out.Reason = "inventory_configuration"
	out.Eligible = v.ManageInventory && !v.AllowBackorder && len(links) == 1
	var sharing []struct{ ID string }
	var item struct{ ID string }
	if len(links) == 1 {
		out.InventoryItemID = links[0].InventoryItemID
		// The relational column is DECIMAL(38,18); compare exactly, never float64.
		out.Eligible = out.Eligible && links[0].RequiredQuantity == "1.000000000000000000"
		e := tx.Table("inventory_items").Select("id").Where("id=? AND deleted_at IS NULL", out.InventoryItemID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&item).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return out, ErrUnavailable
		}
		out.Eligible = out.Eligible && item.ID != ""
		if tx.Table("catalog_variant_inventory_items l").Select("l.id").Joins("JOIN catalog_variants v ON v.id=l.variant_id AND v.deleted_at IS NULL").Where("l.inventory_item_id=? AND l.variant_id<>? AND l.deleted_at IS NULL", out.InventoryItemID, variantID).Order("l.id").Limit(1).Clauses(clause.Locking{Strength: "SHARE"}).Find(&sharing).Error != nil {
			return out, ErrUnavailable
		}
		if len(sharing) > 0 {
			out.Eligible = false
			out.Reason = "shared_inventory_item"
		}
	}
	var tracking struct{ ID, InventoryItemID string }
	e := tx.Table("device_variant_tracking").Select("id,inventory_item_id").Where("variant_id=? AND deleted_at IS NULL", variantID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&tracking).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, ErrUnavailable
	}
	out.Enabled = tracking.ID != ""
	if out.Enabled {
		out.Eligible = false
		out.Reason = "already_enabled"
	} else if out.Eligible {
		out.Reason = ""
	}
	var gate struct {
		Revision        uint64
		CommandsEnabled bool
	}
	if tx.Table("device_runtime_schema").Where("id=1").Take(&gate).Error != nil || !gate.CommandsEnabled || gate.Revision == 0 || gate.Revision == math.MaxUint64 {
		return out, ErrUnavailable
	}
	b, _ := json.Marshal([]any{productID, variantID, v, links, item.ID, sharing, tracking, gate.Revision})
	digest := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(digest[:])
	return out, nil
}
func (h Handler) ReadTracking(ctx context.Context, actor identity.User, productID, variantID string) (Tracking, error) {
	var out Tracking
	if !validPathID(productID) || !validPathID(variantID) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := trackingAuthority(ctx, tx, actor, false); e != nil {
			return e
		}
		if e := LockIdentityTx(ctx, tx); e != nil {
			return e
		}
		var e error
		out, e = inspectTracking(tx, productID, variantID)
		return e
	})
	return out, e
}
func validPathID(v string) bool {
	return validID(v) && strings.TrimSpace(v) == v && !strings.ContainsAny(v, "/\\?#")
}
func validateEnroll(in EnrollInput) error {
	id, e := uuid.Parse(in.OperationID)
	b, e2 := hex.DecodeString(in.ExpectedRevision)
	if e != nil || id.Version() != 4 || id.String() != in.OperationID || e2 != nil || len(b) != 32 || hex.EncodeToString(b) != in.ExpectedRevision {
		return ErrInput
	}
	return nil
}
func (h Handler) Enroll(ctx context.Context, actor identity.User, productID, variantID string, in EnrollInput) (EnrollResult, error) {
	var out EnrollResult
	if !validPathID(productID) || !validPathID(variantID) || validateEnroll(in) != nil {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	request, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := trackingAuthority(ctx, tx, actor, true); e != nil {
			return e
		}
		if e := LockIdentityTx(ctx, tx); e != nil {
			return e
		}
		current, e := inspectTracking(tx, productID, variantID)
		if e != nil {
			return e
		}
		var prior struct{ ActorID, ProductID, VariantID, RequestJSON, ResultJSON string }
		e = tx.Table("device_tracking_changes").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ActorID != actor.ID || prior.ProductID != productID || prior.VariantID != variantID || prior.RequestJSON != string(request) {
				return ErrConflict
			}
			if json.Unmarshal([]byte(prior.ResultJSON), &out) != nil || out.Tracking.ProductID != productID || out.Tracking.VariantID != variantID || !out.Tracking.Enabled {
				return ErrUnavailable
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if current.Revision != in.ExpectedRevision || !current.Eligible || current.Enabled {
			return ErrConflict
		}
		if tx.Exec("INSERT INTO device_variant_tracking(id,variant_id,inventory_item_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "dvar_"+uuid.NewString(), variantID, current.InventoryItemID).Error != nil {
			return ErrUnavailable
		}
		data, _ := json.Marshal(map[string]string{"variant_id": variantID})
		if tx.Exec("INSERT INTO device_events(id,actor_id,action,data,created_at,updated_at) VALUES(?,?,'enroll',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), actor.ID, string(data)).Error != nil {
			return ErrUnavailable
		}
		if tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		after, e := inspectTracking(tx, productID, variantID)
		if e != nil {
			return e
		}
		out = EnrollResult{Tracking: after}
		result, _ := json.Marshal(out)
		if tx.Exec("INSERT INTO device_tracking_changes(operation_id,actor_id,product_id,variant_id,request_json,result_json,created_at) VALUES(?,?,?,?,?,?,UTC_TIMESTAMP(6))", in.OperationID, actor.ID, productID, variantID, string(request), string(result)).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}
