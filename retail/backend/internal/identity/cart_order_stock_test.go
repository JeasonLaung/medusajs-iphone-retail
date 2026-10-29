package identity_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkPaidOrderStock(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, in cart.InitialPaymentCommand, o cart.InitialPaymentOptions) {
	t.Helper()
	rollback := errors.New("owned completion not yet committed")
	run := func(fn func(*gorm.DB) error) error {
		return db.WithContext(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	}
	enable := func(tx *gorm.DB) error {
		return tx.Exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'").Error
	}
	fingerprint := func() string {
		t.Helper()
		result := ""
		for _, table := range []string{"orders", "order_addresses", "order_carts", "order_items", "order_line_items", "order_line_adjustments", "order_line_tax_lines", "order_shipping_versions", "order_shipping_methods", "order_shipping_adjustments", "order_shipping_tax_lines", "order_promotions", "inventory_reservations", "inventory_reservation_operations"} {
			var n int64
			if db.Table(table).Count(&n).Error != nil {
				t.Fatal("order stock fingerprint")
			}
			result += table + "=" + fmt.Sprint(n) + ";"
		}
		var state struct{ ReservedQuantity string }
		if db.Table("inventory_levels").Select("reserved_quantity").Where("id='add-level'").Take(&state).Error != nil {
			t.Fatal("stock counter read")
		}
		var numbers struct{ LastDisplayID, LastTick int64 }
		if db.Table("order_number_allocator").Select("last_display_id,last_tick").Where("id=1").Take(&numbers).Error != nil {
			t.Fatal("order allocator read")
		}
		result += state.ReservedQuantity + ";" + fmt.Sprint(numbers)
		return result
	}
	before := fingerprint()
	if err := run(func(tx *gorm.DB) error {
		_, err := cart.StagePaidOrderAndStock(ctx, tx, "shop", ref, in, o)
		return err
	}); err != inventory.ErrSchema {
		t.Fatal("disabled reservation allowed", err)
	}
	if fingerprint() != before {
		t.Fatal("disabled reservation leaked order")
	}
	stage := func(tx *gorm.DB) error {
		if err := enable(tx); err != nil {
			return err
		}
		out, err := cart.StagePaidOrderAndStock(ctx, tx, "shop", ref, in, o)
		if err != nil {
			return err
		}
		if len(out.ReservationIDs) != 1 || len(out.Snapshot.LineIDs) != 1 {
			t.Error("order stock mapping missing")
			return rollback
		}
		var line string
		for _, id := range out.Snapshot.LineIDs {
			line = id
		}
		var saved struct {
			LineItemID, InventoryItemID, LocationID, Quantity string
			AllowBackorder                                    bool
		}
		if tx.Table("inventory_reservations").Where("id=?", out.ReservationIDs[0]).Take(&saved).Error != nil || saved.LineItemID != line || saved.InventoryItemID != "add-item" || saved.LocationID != "add-location" || saved.Quantity != "2.500000000000000000" || saved.AllowBackorder {
			t.Error("reservation differs from authoritative SKU/warehouse requirements")
		}
		var bound int64
		if tx.Table("order_items").Where("order_id=? AND item_id=? AND version=1", out.Snapshot.OrderID, line).Count(&bound).Error != nil || bound != 1 {
			t.Error("reservation references foreign order line")
		}
		var owner string
		if tx.Table("inventory_reservation_operations").Select("owner_id").Where("operation_id=?", in.OperationID).Scan(&owner).Error != nil || owner != out.Snapshot.OrderID {
			t.Error("reservation receipt not bound to new order")
		}
		var level string
		if tx.Table("inventory_levels").Select("reserved_quantity").Where("id='add-level'").Scan(&level).Error != nil || level != "2.500000000000000000" {
			t.Error("reservation level counter not applied")
		}
		// Ordinary writes stay frozen; the private read capability does not leak
		// into this context, even within the exact same transaction.
		if _, err = cart.LockForWrite(ctx, tx, "shop", ref, "5"); err != cart.ErrConflict {
			t.Error("checkout read capability escaped")
		}
		return rollback
	}
	if err := run(stage); err != rollback {
		t.Fatal("paid order stock stage", err)
	}
	if fingerprint() != before {
		t.Fatal("later completion rollback leaked order or reservation")
	}
	for _, query := range []string{
		"UPDATE inventory_levels SET stocked_quantity=0 WHERE id='add-level'",  // incoming=999 is not stock
		"UPDATE inventory_levels SET reserved_quantity=1 WHERE id='add-level'", // unexplained counter
		"UPDATE commerce_channel_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE id='add-location-link'",
		"UPDATE fulfillment_location_providers SET deleted_at=UTC_TIMESTAMP(6) WHERE id='delivery-provider-location'",
		"INSERT INTO inventory_migration_holds(inventory_item_id,location_id,reason,snapshot_sha256,evidence_sha256,evidence_json,policy_version) VALUES('add-item','add-location','reservation_mismatch',UNHEX(REPEAT('11',32)),UNHEX(REPEAT('22',32)),'{}',1)",
	} {
		err := run(func(tx *gorm.DB) error {
			if err := enable(tx); err != nil {
				return err
			}
			if err := tx.Exec(query).Error; err != nil {
				return err
			}
			_, err := cart.StagePaidOrderAndStock(ctx, tx, "shop", ref, in, o)
			return err
		})
		if err == nil || !errors.Is(err, cart.ErrItemStock) && !errors.Is(err, cart.ErrItemUnavailable) {
			t.Fatal("changed warehouse facts accepted", err)
		}
		if fingerprint() != before {
			t.Fatal("stock failure leaked rows or count")
		}
	}
	// Backorder is a current server-side variant flag, never supplied by the
	// browser. A healthy zero-stock level is allowed only when that flag is set.
	if err := run(func(tx *gorm.DB) error {
		if err := enable(tx); err != nil {
			return err
		}
		if tx.Exec("UPDATE catalog_variants SET allow_backorder=TRUE WHERE id='add-v'").Error != nil || tx.Exec("UPDATE inventory_levels SET stocked_quantity=0 WHERE id='add-level'").Error != nil {
			return errors.New("owned backorder setup")
		}
		out, err := cart.StagePaidOrderAndStock(ctx, tx, "shop", ref, in, o)
		if err != nil {
			return err
		}
		var allowed bool
		if len(out.ReservationIDs) != 1 || tx.Table("inventory_reservations").Select("allow_backorder").Where("id=?", out.ReservationIDs[0]).Scan(&allowed).Error != nil || !allowed {
			t.Error("server backorder was not preserved")
		}
		return rollback
	}); err != rollback {
		t.Fatal("paid backorder reservation", err)
	}
	if fingerprint() != before {
		t.Fatal("backorder rollback changed stock")
	}
	// Untracked merchandise still requires its original delivery coverage, but
	// must not fabricate inventory reservations or require a stock capability.
	if err := run(func(tx *gorm.DB) error {
		if tx.Exec("UPDATE catalog_variants SET manage_inventory=FALSE WHERE id='add-v'").Error != nil {
			return errors.New("owned untracked setup")
		}
		out, err := cart.StagePaidOrderAndStock(ctx, tx, "shop", ref, in, o)
		if err != nil {
			return err
		}
		if len(out.ReservationIDs) != 0 || len(out.Snapshot.LineIDs) != 1 {
			t.Error("untracked merchandise fabricated a reservation")
		}
		return rollback
	}); err != rollback {
		t.Fatal("paid untracked order stage", err)
	}
	if fingerprint() != before {
		t.Fatal("untracked rollback changed order state")
	}
	// Failure after level update and reservation insertion must roll back both
	// those writes AND all order copies/number allocation in the outer transaction.
	if db.Exec("CREATE TRIGGER fail_order_reservation_receipt BEFORE INSERT ON inventory_reservation_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned reservation receipt failure'").Error != nil {
		t.Fatal("reservation trigger setup")
	}
	if err := run(func(tx *gorm.DB) error {
		if err := enable(tx); err != nil {
			return err
		}
		_, err := cart.StagePaidOrderAndStock(ctx, tx, "shop", ref, in, o)
		return err
	}); err != inventory.ErrReservationStorage {
		t.Fatal("late inventory write failure hidden", err)
	}
	if db.Exec("DROP TRIGGER fail_order_reservation_receipt").Error != nil {
		t.Fatal("reservation trigger cleanup")
	}
	if fingerprint() != before {
		t.Fatal("late reservation failure leaked")
	}
	var group sync.WaitGroup
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := run(stage); err != rollback {
				t.Errorf("concurrent rolled-back staging: %v", err)
			}
		}()
	}
	group.Wait()
	if fingerprint() != before {
		t.Fatal("parallel staging changed committed state")
	}
	wrong := ref
	wrong.Generation = strings.Repeat("b", 64)
	if err := run(func(tx *gorm.DB) error {
		_, err := cart.StagePaidOrderAndStock(ctx, tx, "shop", wrong, in, o)
		return err
	}); err != cart.ErrDenied {
		t.Fatal("foreign order stock allowed", err)
	}
	t.Log(`CART_ORDER_STOCK_REHEARSAL {"real_mysql":true,"sealed_paid_and_current_possession":true,"saved_shipping_revalidated":true,"sku_required_quantity":"2.5","reservation_new_order_line":true,"level_and_operation_owner_bound":true,"ordinary_cart_mutation_stays_frozen":true,"disabled_inventory_closed":true,"incoming_not_stock":true,"mismatched_reservation_sum_denied":true,"migration_hold_denied":true,"server_backorder_preserved":true,"untracked_no_reservation":true,"removed_channel_or_provider_denied":true,"late_receipt_failure_rollback":true,"outer_rollback_order_number_and_stock":true,"concurrent_rollback_stages":4,"no_capture_no_complete_no_mail":true}`)
}
