package buyback

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/identity"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrDeviceMismatch = errors.New("buyback inspection device mismatch")
var ErrLinkDevices = errors.New("buyback devices must be linked before quoting")

// InspectionTx must run in the caller's transaction. Request locking precedes
// device locking; device uniqueness is additionally enforced by the database.
func InspectionTx(ctx context.Context, tx *gorm.DB, actor identity.User, store, id string, in InspectionCommand) (AdminDetail, error) {
	var out AdminDetail
	var e error
	out.Enabled, e = adminGate(ctx, tx, actor, true)
	if e != nil {
		return out, e
	}
	raw, e := json.Marshal(in)
	if e != nil {
		return out, ErrInput
	}
	in, e = ParseInspectionCommand(raw)
	if e != nil {
		return out, e
	}
	account, e := adminOwner(tx, store, id, true)
	if e != nil {
		return out, e
	}
	request, e := ReadRequest(ctx, tx, store, account, id)
	if e != nil {
		return out, e
	}
	if request.Status != "received" {
		return out, ErrCommandState
	}
	changes := map[string]any{}
	status, notice := request.Status, false
	switch in.Action {
	case "link_device":
		index := -1
		for i, item := range request.Items {
			if item.ID == in.ItemID {
				index = i
			}
		}
		if index < 0 || request.Items[index].DeviceID != nil {
			return out, ErrDeviceMismatch
		}
		item := request.Items[index]
		if e := lockInspectionDevice(tx, store, in.DeviceID, item, "UPDATE"); e != nil {
			return out, e
		}
		var existing int64
		if tx.Table("buyback_device_links").Where("deleted_at IS NULL AND (device_id=? OR item_id=?)", in.DeviceID, item.ID).Count(&existing).Error != nil {
			return out, ErrUnavailable
		}
		if existing != 0 {
			return out, ErrDeviceMismatch
		}
		now := time.Now().UTC()
		if e := tx.Table("buyback_device_links").Create(map[string]any{"id": "bbl_" + uuid.NewString(), "request_id": id, "item_id": item.ID, "device_id": in.DeviceID, "created_at": now, "updated_at": now}).Error; e != nil {
			var duplicate *mysqlDriver.MySQLError
			if errors.As(e, &duplicate) && duplicate.Number == 1062 {
				return out, ErrDeviceMismatch
			}
			return out, ErrUnavailable
		}
		request.Items[index].DeviceID = &in.DeviceID
	case "final_offer":
		if len(in.Items) != len(request.Items) {
			return out, ErrInput
		}
		byID := map[string]InspectionOffer{}
		for _, item := range in.Items {
			if _, exists := byID[item.ItemID]; exists {
				return out, ErrInput
			}
			byID[item.ItemID] = item
		}
		var sum float64
		for i, item := range request.Items {
			if item.DeviceID == nil {
				return out, ErrLinkDevices
			}
			var links []struct{ ID string }
			if tx.Table("buyback_device_links").Select("id").Where("request_id=? AND item_id=? AND device_id=? AND deleted_at IS NULL", id, item.ID, *item.DeviceID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(2).Find(&links).Error != nil {
				return out, ErrUnavailable
			}
			if len(links) != 1 {
				return out, ErrLinkDevices
			}
			offer, exists := byID[item.ID]
			if !exists {
				return out, ErrInput
			}
			request.Items[i].FinalAmount = &offer.Amount
			request.Items[i].InspectionNotes = offer.Notes
			amount, _ := offer.Amount.Float64()
			sum += amount
		}
		// Preserve original ordered binary64 sum, rounded once to cents.
		amount := strconv.FormatFloat(math.Round(sum*100)/100, 'f', -1, 64)
		changes["final_amount"] = amount
		rawAmount, _ := json.Marshal(map[string]any{"value": amount, "precision": 100})
		changes["raw_final_amount"] = string(rawAmount)
		status, notice = "final_offer", true
	}
	items, e := json.Marshal(request.Items)
	if e != nil {
		return out, ErrUnavailable
	}
	changes["items"] = string(items)
	return finishAdminCommandTx(ctx, tx, actor, store, account, request, out, in.Action, "", "", status, changes, notice)
}

func lockInspectionDevice(tx *gorm.DB, store, id string, item RequestItem, strength string) error {
	var device struct{ ID, VariantID, SerialNumber, IMEI, Category, State, LocationID string }
	e := tx.Table("devices").Select("id,variant_id,serial_number,imei,category,state,location_id").Where("id=? AND deleted_at IS NULL", id).Clauses(clause.Locking{Strength: strength}).Take(&device).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return ErrDeviceMismatch
	}
	if e != nil {
		return ErrUnavailable
	}
	if device.Category != "recycled" || device.State != "inspection" || item.SerialNumber == "" || device.SerialNumber != item.SerialNumber || device.IMEI != item.IMEI || !commandIMEI.MatchString(item.IMEI) {
		return ErrDeviceMismatch
	}
	// Reuse the configured store/channel relation, including current warehouse
	// and source ownership. IDs or matching identities alone never grant scope.
	var owned []struct{ ID string }
	if tx.Table("commerce_stores s").Select("l.id").Joins("JOIN commerce_sales_channels c ON c.id=s.default_sales_channel_id AND c.deleted_at IS NULL AND c.is_disabled=FALSE").Joins("JOIN commerce_channel_locations cl ON cl.sales_channel_id=c.id AND cl.deleted_at IS NULL").Joins("JOIN inventory_locations l ON l.id=cl.stock_location_id AND l.deleted_at IS NULL").Where("s.id=? AND s.deleted_at IS NULL AND l.id=?", store, device.LocationID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(2).Scan(&owned).Error != nil {
		return ErrUnavailable
	}
	if len(owned) != 1 {
		return ErrDeviceMismatch
	}
	var source struct{ ID string }
	if tx.Table("device_sources s").Select("s.id").Joins("JOIN devices d ON d.source_id=s.id AND d.location_id=s.location_id").Where("d.id=? AND s.deleted_at IS NULL", id).Clauses(clause.Locking{Strength: "SHARE"}).Take(&source).Error != nil {
		return ErrDeviceMismatch
	}
	var variant struct{ ID string }
	if tx.Table("catalog_variants v").Select("v.id").Joins("JOIN catalog_products p ON p.id=v.product_id AND p.deleted_at IS NULL").Where("v.id=? AND v.product_id=? AND v.deleted_at IS NULL", device.VariantID, item.ProductID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&variant).Error != nil {
		return ErrDeviceMismatch
	}
	var options []struct{ Value string }
	if tx.Table("catalog_variant_options l").Select("v.value").Joins("JOIN catalog_option_values v ON v.id=l.option_value_id AND v.deleted_at IS NULL").Joins("JOIN catalog_options o ON o.id=v.option_id AND o.deleted_at IS NULL").Where("l.variant_id=?", variant.ID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(101).Scan(&options).Error != nil || len(options) > 100 {
		return ErrUnavailable
	}
	for _, option := range options {
		// Original link workflow removes JS whitespace (not storage aliases).
		value := strings.Map(func(r rune) rune {
			if strings.ContainsRune(jsWhitespace, r) {
				return -1
			}
			return r
		}, option.Value)
		if strings.ToUpper(value) == item.Capacity {
			return nil
		}
	}
	return ErrDeviceMismatch
}

type InspectionCandidates struct {
	Devices []string `json:"devices"`
}

// Only exact identity matches are offered; stale selections are rechecked by
// the write transaction. This does not register or change a physical device.
func InspectionCandidatesTx(ctx context.Context, tx *gorm.DB, actor identity.User, store, id, itemID string) (InspectionCandidates, error) {
	out := InspectionCandidates{Devices: []string{}}
	if _, e := adminGate(ctx, tx, actor, false); e != nil {
		return out, e
	}
	account, e := adminOwner(tx, store, id, false)
	if e != nil {
		return out, e
	}
	request, e := ReadRequest(ctx, tx, store, account, id)
	if e != nil {
		return out, e
	}
	if request.Status != "received" {
		return out, ErrCommandState
	}
	var selected *RequestItem
	for i := range request.Items {
		if request.Items[i].ID == itemID {
			selected = &request.Items[i]
		}
	}
	if selected == nil {
		return out, ErrMissing
	}
	if selected.DeviceID != nil {
		return out, nil
	}
	var matches []struct{ ID string }
	if tx.Table("devices d").Select("d.id").Where("d.serial_number=? AND d.imei=? AND d.deleted_at IS NULL AND d.category='recycled' AND d.state='inspection'", selected.SerialNumber, selected.IMEI).Where("NOT EXISTS(SELECT 1 FROM buyback_device_links l WHERE l.device_id=d.id AND l.deleted_at IS NULL)").Order("d.id").Limit(101).Scan(&matches).Error != nil || len(matches) > 100 {
		return out, ErrUnavailable
	}
	for _, device := range matches {
		if e := lockInspectionDevice(tx, store, device.ID, *selected, "SHARE"); e == nil {
			out.Devices = append(out.Devices, device.ID)
		} else if !errors.Is(e, ErrDeviceMismatch) {
			return out, e
		}
	}
	return out, nil
}
