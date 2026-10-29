package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"mime"
	"sort"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Packing reuses the explicit allocation action. It creates the parcel and
// consumes quantity inventory atomically; preallocation alone never does that.
type PackingDevice struct {
	AssignmentID string    `json:"assignment_id"`
	DeviceID     string    `json:"device_id"`
	SerialNumber string    `json:"serial_number"`
	ItemID       string    `json:"item_id"`
	Title        string    `json:"title"`
	SourceID     string    `json:"source_id"`
	SourceName   string    `json:"source_name"`
	LocationID   string    `json:"location_id"`
	LocationName string    `json:"location_name"`
	VariantID    string    `json:"-"`
	UpdatedAt    time.Time `json:"-"`
}
type PackingOption struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	ProviderID    string  `json:"provider_id"`
	Supported     bool    `json:"supported"`
	ProfileID     *string `json:"-"`
	ServiceZoneID string  `json:"-"`
}
type PackingContext struct {
	OrderID     string          `json:"order_id"`
	Enabled     bool            `json:"enabled"`
	Paid        bool            `json:"paid"`
	OrderActive bool            `json:"order_active"`
	Devices     []PackingDevice `json:"devices"`
	Options     []PackingOption `json:"options"`
	Revision    string          `json:"revision"`
}
type PackingResult struct {
	FulfillmentID string `json:"fulfillment_id"`
	Replayed      bool   `json:"replayed"`
}

// The strict allocation wire format is reused: action must be "reserve" here,
// ids are current un-packed assignment IDs. Shipping uses the order's first
// current shipping method, matching save_allocation in the original workflow.
type packingStock struct {
	ID                  string    `json:"id"`
	InventoryItemID     string    `json:"inventory_item_id"`
	LocationID          string    `json:"location_id"`
	StockedQuantity     string    `json:"stocked_quantity"`
	ReservedQuantity    string    `json:"reserved_quantity"`
	IncomingQuantity    string    `json:"incoming_quantity"`
	RawStockedQuantity  *string   `json:"raw_stocked_quantity"`
	RawReservedQuantity *string   `json:"raw_reserved_quantity"`
	RawIncomingQuantity *string   `json:"raw_incoming_quantity"`
	UpdatedAt           time.Time `json:"updated_at"`
}
type packingReservation struct {
	ID              string    `json:"id"`
	InventoryItemID string    `json:"inventory_item_id"`
	LocationID      string    `json:"location_id"`
	LineItemID      *string   `json:"line_item_id"`
	Quantity        string    `json:"quantity"`
	RawQuantity     string    `json:"raw_quantity"`
	AllowBackorder  *bool     `json:"allow_backorder"`
	UpdatedAt       time.Time `json:"updated_at"`
}
type packingFacts struct {
	AddressID    *string
	Version      int
	Address      map[string]any
	Lines        map[string]AllocationContext
	Stock        []packingStock
	Reservations []packingReservation
}

func packingGate(ctx context.Context, tx *gorm.DB, actor identity.User) (uint64, bool, error) {
	rev, enabled, e := allocationGate(ctx, tx, actor)
	if e != nil {
		return 0, false, e
	}
	if e = authorization.CheckTx(ctx, tx, actor, authorization.OrderFulfillmentRead); e != nil {
		return 0, false, e
	}
	var cap struct {
		Version int
		Enabled bool
	}
	if tx.Table("order_device_packing_runtime").Where("id=1").Clauses(clause.Locking{Strength: "SHARE"}).Take(&cap).Error != nil || cap.Version != 1 || fulfillment.Ready(ctx, tx) != nil {
		return 0, false, ErrUnavailable
	}
	// All existing stock writers lock this marker for SHARE before stock rows.
	// Exclusive ownership here permits moving/splitting one order reservation
	// between warehouses without racing checkout, stock edits or another parcel.
	var marker struct{ ID int }
	if tx.Table("inventory_runtime_schema").Select("id").Where("id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&marker).Error != nil || inventory.Ready(ctx, tx) != nil {
		return 0, false, ErrUnavailable
	}
	return rev, enabled && cap.Enabled, nil
}
func loadPacking(tx *gorm.DB, o string, rev uint64, enabled bool) (PackingContext, packingFacts, error) {
	out := PackingContext{OrderID: o, Enabled: enabled, Devices: []PackingDevice{}, Options: []PackingOption{}}
	facts := packingFacts{Lines: map[string]AllocationContext{}}
	var order struct {
		ID, Status        string
		Version           int
		IsDraftOrder      bool
		CanceledAt        *time.Time
		ShippingAddressID *string
		UpdatedAt         time.Time
	}
	if e := tx.Table("orders").Select("id,status,version,is_draft_order,canceled_at,shipping_address_id,updated_at").Where("id=? AND deleted_at IS NULL", o).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&order).Error; e != nil {
		return out, facts, e
	}
	facts.Version = order.Version
	facts.AddressID = order.ShippingAddressID
	out.OrderActive = !order.IsDraftOrder && order.Status != "draft" && order.Status != "canceled" && order.CanceledAt == nil
	if order.ShippingAddressID != nil {
		facts.Address = map[string]any{}
		if tx.Table("order_addresses").Where("id=? AND deleted_at IS NULL", *order.ShippingAddressID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&facts.Address).Error != nil {
			return out, facts, ErrConflict
		}
	}
	var payment []struct {
		ID        string
		Status    *string
		UpdatedAt *time.Time
	}
	if tx.Table("order_payment_collections l").Select("l.id,p.status,p.updated_at").Joins("LEFT JOIN payment_collections p ON p.id=l.payment_collection_id AND p.deleted_at IS NULL").Where("l.order_id=? AND l.deleted_at IS NULL", o).Order("l.id").Limit(101).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&payment).Error != nil || len(payment) > 100 {
		return out, facts, ErrUnavailable
	}
	out.Paid = len(payment) > 0
	for _, p := range payment {
		out.Paid = out.Paid && p.Status != nil && *p.Status == "completed"
	}
	if tx.Table("device_assignments a").Select("a.id AS assignment_id,d.id AS device_id,d.serial_number,a.item_id,li.title,d.source_id,s.name AS source_name,d.location_id,l.name AS location_name,d.variant_id,d.updated_at").Joins("JOIN devices d ON d.id=a.device_id AND d.deleted_at IS NULL AND d.state='reserved' AND d.category='sale'").Joins("JOIN device_sources s ON s.id=d.source_id AND s.deleted_at IS NULL AND s.location_id=d.location_id AND a.source_id=s.id").Joins("JOIN inventory_locations l ON l.id=d.location_id AND l.deleted_at IS NULL").Joins("JOIN order_items oi ON oi.item_id=a.item_id AND oi.order_id=a.order_id AND oi.version=? AND oi.deleted_at IS NULL", order.Version).Joins("JOIN order_line_items li ON li.id=a.item_id AND li.variant_id=d.variant_id AND li.deleted_at IS NULL").Where("a.order_id=? AND a.status='reserved' AND a.fulfillment_id IS NULL AND a.deleted_at IS NULL", o).Order("a.id").Limit(2001).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&out.Devices).Error != nil || len(out.Devices) > 2000 {
		return out, facts, ErrUnavailable
	}
	// Keep original ordering of current shipping methods. The first method is
	// authoritative, including unsupported providers; never silently choose a
	// different manual option or invent a provider booking.
	var methods []struct{ ShippingOptionID *string }
	if tx.Table("order_shipping_versions v").Select("m.shipping_option_id").Joins("JOIN order_shipping_methods m ON m.id=v.shipping_method_id AND m.deleted_at IS NULL").Where("v.order_id=? AND v.version=? AND v.deleted_at IS NULL", o, order.Version).Order("m.created_at,m.id").Limit(101).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&methods).Error != nil || len(methods) > 100 {
		return out, facts, ErrUnavailable
	}
	if len(methods) > 0 && methods[0].ShippingOptionID != nil {
		var option PackingOption
		e := tx.Table("fulfillment_shipping_options o").Select("o.id,o.name,o.provider_id,o.shipping_profile_id AS profile_id,o.service_zone_id").Joins("JOIN fulfillment_providers p ON p.id=o.provider_id AND p.is_enabled=TRUE AND p.deleted_at IS NULL").Where("o.id=? AND o.deleted_at IS NULL", *methods[0].ShippingOptionID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&option).Error
		if e == nil {
			option.Supported = option.ProviderID == "manual_manual"
			out.Options = append(out.Options, option)
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return out, facts, e
		}
	}
	for _, d := range out.Devices {
		if _, ok := facts.Lines[d.ItemID]; !ok {
			c, e := loadAllocation(tx, o, d.ItemID, rev, enabled)
			if e != nil {
				return out, facts, e
			}
			facts.Lines[d.ItemID] = c
		}
	}
	// Stock/hold/reservation configuration participates in the optimistic revision.
	// The marker lock above makes these reads current and stable through commit.
	var stock []packingStock
	if tx.Raw("SELECT l.id,l.inventory_item_id,l.location_id,l.stocked_quantity,l.reserved_quantity,l.incoming_quantity,l.raw_stocked_quantity,l.raw_reserved_quantity,l.raw_incoming_quantity,l.updated_at FROM inventory_levels l WHERE l.deleted_at IS NULL AND l.inventory_item_id IN (SELECT vi.inventory_item_id FROM catalog_variant_inventory_items vi JOIN order_line_items li ON li.variant_id=vi.variant_id JOIN order_items oi ON oi.item_id=li.id AND oi.order_id=? AND oi.version=? AND oi.deleted_at IS NULL WHERE vi.deleted_at IS NULL AND li.deleted_at IS NULL) ORDER BY l.id LIMIT 10001 FOR SHARE", o, order.Version).Scan(&stock).Error != nil || len(stock) > 10000 {
		return out, facts, ErrUnavailable
	}
	facts.Stock = stock
	var reservations []packingReservation
	if tx.Raw("SELECT r.id,r.inventory_item_id,r.location_id,r.line_item_id,r.quantity,r.raw_quantity,r.allow_backorder,r.updated_at FROM inventory_reservations r JOIN order_items oi ON oi.item_id=r.line_item_id AND oi.order_id=? AND oi.version=? AND oi.deleted_at IS NULL WHERE r.deleted_at IS NULL ORDER BY r.id LIMIT 10001 FOR SHARE", o, order.Version).Scan(&reservations).Error != nil || len(reservations) > 10000 {
		return out, facts, ErrUnavailable
	}
	facts.Reservations = reservations
	data, e := json.Marshal([]any{order, facts.Address, out.Devices, out.Options, methods, payment, facts.Lines, stock, reservations, rev, func() any {
		if len(out.Options) == 1 {
			return []any{out.Options[0].ProfileID, out.Options[0].ServiceZoneID}
		}
		return nil
	}()})
	if e != nil {
		return out, facts, ErrUnavailable
	}
	hash := sha256.Sum256(data)
	out.Revision = hex.EncodeToString(hash[:])
	return out, facts, nil
}
func (h Handler) ReadPacking(ctx context.Context, actor identity.User, o string) (PackingContext, error) {
	var out PackingContext
	if h.DB == nil {
		return out, ErrUnavailable
	}
	if !validPathID(o) {
		return out, ErrInput
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rev, enabled, e := packingGate(ctx, tx, actor)
		if e != nil {
			return e
		}
		out, _, e = loadPacking(tx, o, rev, enabled)
		return e
	})
	return out, e
}
func packingExactlyOne(query *gorm.DB, column string) bool {
	var ids []string
	return query.Limit(2).Clauses(clause.Locking{Strength: "SHARE"}).Pluck(column, &ids).Error == nil && len(ids) == 1
}

func rawCount(s string) string {
	b, _ := json.Marshal(map[string]any{"value": s, "precision": 38})
	return string(b)
}
func nonnegative(s string) bool { n, ok := new(big.Rat).SetString(s); return ok && n.Sign() >= 0 }

// consumePackingStock must run under the inventory marker lock in packingGate.
// It preserves the unfulfilled remainder at the original reservation location.
func consumePackingStock(tx *gorm.DB, item, stockID, location string, units int) error {
	var reservations []struct{ ID, InventoryItemID, LocationID, Quantity string }
	if tx.Table("inventory_reservations").Select("id,inventory_item_id,location_id,quantity").Where("line_item_id=? AND deleted_at IS NULL", item).Order("id").Limit(3).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&reservations).Error != nil || len(reservations) > 1 {
		return ErrConflict
	}
	amount := strconv.Itoa(units)
	var rID, oldLocation, remainder string
	if len(reservations) == 1 {
		r := reservations[0]
		if r.InventoryItemID != stockID {
			return ErrConflict
		}
		rID = r.ID
		oldLocation = r.LocationID
		var e error
		remainder, e = inventory.Difference(r.Quantity, amount)
		if e != nil || !nonnegative(remainder) {
			return ErrConflict
		}
	}
	locations := []string{location}
	if oldLocation != "" && oldLocation != location {
		locations = append(locations, oldLocation)
	}
	sort.Strings(locations)
	for _, loc := range locations {
		var level struct{ ID, StockedQuantity, ReservedQuantity string }
		if tx.Table("inventory_levels").Select("id,stocked_quantity,reserved_quantity").Where("inventory_item_id=? AND location_id=? AND deleted_at IS NULL", stockID, loc).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&level).Error != nil {
			return ErrConflict
		}
		var holds []struct{ Reason string }
		if tx.Table("inventory_migration_holds").Select("reason").Where("inventory_item_id=? AND location_id=?", stockID, loc).Limit(4).Clauses(clause.Locking{Strength: "SHARE"}).Find(&holds).Error != nil || len(holds) != 0 {
			return inventory.ErrStockHeld
		}
		var totals []struct{ Quantity string }
		if tx.Table("inventory_reservations").Select("quantity").Where("inventory_item_id=? AND location_id=? AND deleted_at IS NULL", stockID, loc).Limit(10001).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&totals).Error != nil || len(totals) > 10000 {
			return ErrUnavailable
		}
		sum := "0"
		for _, q := range totals {
			if !nonnegative(q.Quantity) {
				return inventory.ErrStockHeld
			}
			neg, e := inventory.Difference("0", q.Quantity)
			if e != nil {
				return e
			}
			sum, e = inventory.Difference(sum, neg)
			if e != nil {
				return e
			}
		}
		same, e := inventory.Equal(sum, level.ReservedQuantity)
		if e != nil || !same {
			return inventory.ErrStockHeld
		}
		stocked, reserved := level.StockedQuantity, level.ReservedQuantity
		if loc == location {
			stocked, e = inventory.Difference(stocked, amount)
			if e != nil {
				return e
			}
		}
		if loc == oldLocation {
			reserved, e = inventory.Difference(reserved, amount)
			if e != nil {
				return e
			}
		}
		available, e := inventory.Difference(stocked, reserved)
		if e != nil || !nonnegative(available) || !nonnegative(stocked) || !nonnegative(reserved) {
			return inventory.ErrInsufficientStock
		}
		if tx.Exec("UPDATE inventory_levels SET stocked_quantity=?,raw_stocked_quantity=?,reserved_quantity=?,raw_reserved_quantity=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", stocked, rawCount(stocked), reserved, rawCount(reserved), level.ID).Error != nil {
			return ErrUnavailable
		}
	}
	if rID != "" {
		zero, e := inventory.Equal(remainder, "0")
		if e != nil {
			return e
		}
		if zero {
			if tx.Exec("UPDATE inventory_reservations SET deleted_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6) WHERE id=?", rID).Error != nil {
				return ErrUnavailable
			}
		} else if tx.Exec("UPDATE inventory_reservations SET quantity=?,raw_quantity=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", remainder, rawCount(remainder), rID).Error != nil {
			return ErrUnavailable
		}
	}
	return nil
}
func (h Handler) Pack(ctx context.Context, actor identity.User, o string, in AllocationInput) (PackingResult, error) {
	var result PackingResult
	if !validPathID(o) || !validAllocation(in) || in.Action != "reserve" {
		return result, ErrInput
	}
	if h.DB == nil {
		return result, ErrUnavailable
	}
	request, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rev, enabled, e := packingGate(ctx, tx, actor)
		if e != nil {
			return e
		}
		if !enabled {
			return ErrUnavailable
		}
		before, facts, e := loadPacking(tx, o, rev, enabled)
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, OrderID, FulfillmentID, RequestJSON string }
		e = tx.Table("order_device_packings").Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.OrderID != o || receipt.RequestJSON != string(request) {
				return ErrConflict
			}

			if !packingExactlyOne(tx.Table("order_fulfillments l").Joins("JOIN fulfillments f ON f.id=l.fulfillment_id AND f.deleted_at IS NULL").Where("l.order_id=? AND l.fulfillment_id=? AND l.deleted_at IS NULL", o, receipt.FulfillmentID), "l.id") {
				return gorm.ErrRecordNotFound
			}
			for _, id := range in.IDs {

				if !packingExactlyOne(tx.Table("device_assignments a").Joins("JOIN devices d ON d.id=a.device_id AND d.deleted_at IS NULL").Joins("JOIN order_items oi ON oi.order_id=a.order_id AND oi.item_id=a.item_id AND oi.version=? AND oi.deleted_at IS NULL", facts.Version).Joins("JOIN order_line_items li ON li.id=a.item_id AND li.deleted_at IS NULL").Where("a.id=? AND a.order_id=? AND a.fulfillment_id=? AND a.deleted_at IS NULL", id, o, receipt.FulfillmentID), "a.id") {
					return gorm.ErrRecordNotFound
				}
			}
			result = PackingResult{FulfillmentID: receipt.FulfillmentID, Replayed: true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !before.Paid || !before.OrderActive || before.Revision != in.ExpectedRevision || facts.AddressID == nil || len(before.Options) != 1 {
			return ErrConflict
		}
		option := before.Options[0]
		if !option.Supported {
			return ErrUnavailable
		}
		selected := map[string]PackingDevice{}
		for _, d := range before.Devices {
			selected[d.AssignmentID] = d
		}
		var location, source string
		counts := map[string]int{}
		var devices []PackingDevice
		for _, id := range in.IDs {
			d, ok := selected[id]
			if !ok {
				return ErrConflict
			}
			if location == "" {
				location = d.LocationID
				source = d.SourceID
			}
			if d.LocationID != location || d.SourceID != source {
				return ErrConflict
			}
			devices = append(devices, d)
			counts[d.ItemID]++
		}
		// Provider pickup location and fulfillment set must match the actual parcel.

		if !packingExactlyOne(tx.Table("fulfillment_location_providers").Where("stock_location_id=? AND fulfillment_provider_id=? AND deleted_at IS NULL", location, option.ProviderID), "id") {
			return ErrConflict
		}
		if !packingExactlyOne(tx.Table("fulfillment_service_zones z").Joins("JOIN fulfillment_sets s ON s.id=z.fulfillment_set_id AND s.deleted_at IS NULL").Joins("JOIN fulfillment_location_sets l ON l.fulfillment_set_id=s.id AND l.deleted_at IS NULL").Where("z.id=? AND z.deleted_at IS NULL AND l.stock_location_id=?", option.ServiceZoneID, location), "z.id") {
			return ErrConflict
		}
		keys := []string{}
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fulfillmentID := "ful_" + uuid.NewString()
		addressID := "fuladdr_" + uuid.NewString()
		address := map[string]any{"id": addressID, "created_at": time.Now().UTC(), "updated_at": time.Now().UTC()}
		for _, k := range []string{"company", "first_name", "last_name", "address_1", "address_2", "city", "country_code", "province", "postal_code", "phone"} {
			address[k] = facts.Address[k]
		}
		if tx.Table("fulfillment_addresses").Create(address).Error != nil {
			return ErrUnavailable
		}
		metadata, _ := json.Marshal(map[string]any{"device_source_id": source, "device_assignment_ids": in.IDs})
		if tx.Exec("INSERT INTO fulfillments(id,location_id,packed_at,provider_id,shipping_option_id,delivery_address_id,requires_shipping,created_by,metadata,created_at,updated_at) VALUES(?,?,UTC_TIMESTAMP(6),?,?,?,TRUE,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", fulfillmentID, location, option.ProviderID, option.ID, addressID, actor.ID, string(metadata)).Error != nil {
			return ErrUnavailable
		}
		if tx.Exec("INSERT INTO order_fulfillments(id,order_id,fulfillment_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "ordful_"+uuid.NewString(), o, fulfillmentID).Error != nil {
			return ErrUnavailable
		}
		for _, item := range keys {
			c := facts.Lines[item]
			count := counts[item]
			if c.Quantity-c.Fulfilled < count {
				return ErrConflict
			}
			reserved := 0
			for _, a := range c.Assignments {
				if a.Status == "reserved" {
					reserved++
				}
			}
			if reserved > c.Quantity-c.Fulfilled {
				return ErrConflict
			}
			var line struct{ ProductID, VariantID, Title, VariantSKU, VariantBarcode string }
			if tx.Table("order_line_items l").Select("v.product_id,l.variant_id,l.title,COALESCE(l.variant_sku,'') AS variant_sku,COALESCE(l.variant_barcode,'') AS variant_barcode").Joins("JOIN catalog_variants v ON v.id=l.variant_id AND v.deleted_at IS NULL").Where("l.id=? AND l.deleted_at IS NULL", item).Clauses(clause.Locking{Strength: "SHARE"}).Take(&line).Error != nil {
				return ErrConflict
			}
			_, e := intakeSKU(tx, line.ProductID, line.VariantID)
			if e != nil {
				return e
			}
			var stockID string
			if tx.Table("catalog_variant_inventory_items").Select("inventory_item_id").Where("variant_id=? AND deleted_at IS NULL", line.VariantID).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&stockID).Error != nil || stockID == "" {
				return ErrConflict
			}
			if option.ProfileID == nil {
				return ErrConflict
			}
			if !packingExactlyOne(tx.Table("catalog_product_shipping_profiles").Where("product_id=? AND shipping_profile_id=? AND deleted_at IS NULL", line.ProductID, *option.ProfileID), "id") {
				return ErrConflict
			}
			// Channel membership is rechecked for packed reserved devices as well as
			// the original available-device candidate query used by preallocation.
			if !packingExactlyOne(tx.Table("orders o").Joins("JOIN commerce_channel_locations l ON l.sales_channel_id=o.sales_channel_id AND l.deleted_at IS NULL").Where("o.id=? AND l.stock_location_id=?", o, location), "l.id") {
				return ErrConflict
			}
			if e = consumePackingStock(tx, item, stockID, location, count); e != nil {
				return e
			}
			amount := strconv.Itoa(count)
			if tx.Exec("INSERT INTO fulfillment_items(id,fulfillment_id,title,sku,barcode,quantity,raw_quantity,line_item_id,inventory_item_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "fulitem_"+uuid.NewString(), fulfillmentID, line.Title, line.VariantSKU, line.VariantBarcode, amount, rawCount(amount), item, stockID).Error != nil {
				return ErrUnavailable
			}
			var q struct{ ID, FulfilledQuantity string }
			if tx.Table("order_items").Select("id,fulfilled_quantity").Where("order_id=? AND version=? AND item_id=? AND deleted_at IS NULL", o, facts.Version, item).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&q).Error != nil {
				return ErrConflict
			}
			next, e := inventory.Difference(q.FulfilledQuantity, "-"+amount)
			if e != nil {
				return e
			}
			if tx.Exec("UPDATE order_items SET fulfilled_quantity=?,raw_fulfilled_quantity=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", next, rawCount(next), q.ID).Error != nil {
				return ErrUnavailable
			}
		}
		for _, d := range devices {
			var current struct {
				SerialNumber     string
				IMEI, IMEI2, EID *string
			}
			if tx.Table("devices").Select("serial_number,imei,imei2,eid").Where("id=? AND deleted_at IS NULL", d.DeviceID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&current).Error != nil {
				return ErrConflict
			}
			var productID string
			tx.Table("catalog_variants").Select("product_id").Where("id=?", d.VariantID).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&productID)
			sku, e := intakeSKU(tx, productID, d.VariantID)
			if e != nil {
				return e
			}
			ident := Identity{SerialNumber: current.SerialNumber}
			if current.IMEI != nil {
				ident.IMEI = *current.IMEI
			}
			if current.IMEI2 != nil {
				ident.IMEI2 = *current.IMEI2
			}
			if current.EID != nil {
				ident.EID = *current.EID
			}
			normalized, e := NormalizeIdentity(ident, sku.RequiresRadio)
			if e != nil || normalized != ident {
				return ErrIdentity
			}
			if e = ClaimIdentityTx(ctx, tx, d.DeviceID, ident, sku.RequiresRadio); e != nil {
				return e
			}
			if tx.Exec("UPDATE device_assignments SET status='packed',fulfillment_id=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND status='reserved' AND fulfillment_id IS NULL AND deleted_at IS NULL", fulfillmentID, d.AssignmentID).RowsAffected != 1 || tx.Exec("UPDATE devices SET state='packed',updated_at=UTC_TIMESTAMP(6) WHERE id=? AND state='reserved' AND deleted_at IS NULL", d.DeviceID).RowsAffected != 1 {
				return ErrConflict
			}
			event, _ := json.Marshal(map[string]any{"assignment_id": d.AssignmentID, "fulfillment_id": fulfillmentID})
			if tx.Exec("INSERT INTO device_events(id,device_id,order_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,?,'pack',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), d.DeviceID, o, actor.ID, string(event)).Error != nil {
				return ErrUnavailable
			}
		}
		if tx.Exec("UPDATE orders SET updated_at=UTC_TIMESTAMP(6) WHERE id=?", o).Error != nil {
			return ErrUnavailable
		}
		if tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		after, afterFacts, e := loadPacking(tx, o, rev+1, enabled)
		if e != nil {
			return e
		}
		b, _ := json.Marshal(map[string]any{"context": before, "stock": facts.Stock, "reservations": facts.Reservations})
		a, _ := json.Marshal(map[string]any{"context": after, "stock": afterFacts.Stock, "reservations": afterFacts.Reservations})
		if tx.Table("order_device_packings").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "order_id": o, "fulfillment_id": fulfillmentID, "request_json": string(request), "before_json": string(b), "after_json": string(a), "created_at": time.Now().UTC()}).Error != nil {
			return ErrUnavailable
		}
		result.FulfillmentID = fulfillmentID
		return nil
	})
	return result, e
}
func (h Handler) PackingHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	if c.Request.Method != "GET" {
		media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if e != nil || media != "application/json" {
			platform.Fail(c, 415, "devices.content_type", "JSON required")
			return
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	var e error
	if c.Request.URL.RawQuery != "" {
		e = ErrInput
	} else if c.Request.Method == "GET" {
		var out PackingContext
		out, e = h.ReadPacking(ctx, actor, c.Param("id"))
		if e == nil {
			c.JSON(200, out)
			return
		}
	} else {
		var in AllocationInput
		in, e = decodeAllocation(c.Request.Body)
		if e == nil {
			var out PackingResult
			out, e = h.Pack(ctx, actor, c.Param("id"), in)
			if e == nil {
				c.JSON(200, out)
				return
			}
		}
	}
	switch {
	case errors.Is(e, ErrInput), errors.Is(e, ErrIdentity):
		platform.Fail(c, 400, "devices.invalid_packing", "Invalid packing input or identity")
	case errors.Is(e, authorization.ErrDenied):
		platform.Fail(c, 403, "authorization.denied", "Current allocation and fulfillment permissions required")
	case errors.Is(e, gorm.ErrRecordNotFound):
		platform.Fail(c, 404, "orders.not_found", "Order or parcel not found")
	case errors.Is(e, ErrConflict), errors.Is(e, ErrDuplicate), errors.Is(e, inventory.ErrStockHeld), errors.Is(e, inventory.ErrInsufficientStock):
		platform.Fail(c, 409, "devices.packing_conflict", "Payment, order, devices or warehouse stock changed; reload")
	default:
		platform.Fail(c, 503, "devices.packing_unavailable", "Packing unavailable")
	}
}
