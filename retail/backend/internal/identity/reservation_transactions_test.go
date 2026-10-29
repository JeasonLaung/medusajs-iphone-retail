package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"

	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedReservationTransactions(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned MySQL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("stock fixture connection failed")
	}
	base, _ := db.DB()
	defer base.Close()
	if db.Exec("CREATE DATABASE reservation_transaction_rehearsal").Error != nil {
		t.Fatal("stock fixture schema failed")
	}
	cfg.DBName = "reservation_transaction_rehearsal"
	db, err = gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("stock fixture connection failed")
	}
	pool, _ := db.DB()
	defer pool.Close()
	pool.SetMaxOpenConns(10)
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("stock fixture plan failed")
	}
	applied, err := schema.Apply(ctx, pool, plan)
	if err != nil || applied.Applied != 44 {
		t.Fatal("stock fixture migrations failed")
	}
	seedReviewedCatalogFixture(t, db)

	exec := func(query string, args ...any) {
		t.Helper()
		if db.Exec(query, args...).Error != nil {
			t.Fatal("reservation artificial setup failed")
		}
	}
	exec(`INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES ('reserve-location','Reserve fixture',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('reserve-other','Other fixture',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at) VALUES ('reserve-level','fixture-stock','reserve-location',1,0,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('reserve-other-level','fixture-stock','reserve-other',1,0,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	command := func(id, quantity string, backorder bool) inventory.ReservationCommand {
		return inventory.ReservationCommand{OperationID: uuid.NewString(), OwnerID: "fixture-order", ActorRef: "fixture-owner-workflow", Kind: "reserve", Reserve: []inventory.ReservationInput{{ID: id, InventoryItemID: "fixture-stock", LocationID: "reserve-location", LineItemID: "fixture-order-line", Quantity: quantity, AllowBackorder: backorder}}}
	}
	run := func(cmd inventory.ReservationCommand, commit bool) (inventory.ReservationResult, error) {
		tx, err := pool.BeginTx(ctx, nil)
		if err != nil {
			return inventory.ReservationResult{}, err
		}
		defer tx.Rollback()
		result, err := inventory.ApplyReservations(ctx, tx, cmd)
		if err != nil {
			return result, err
		}
		if commit {
			err = tx.Commit()
		}
		return result, err
	}
	count := func(table string) int64 {
		var n int64
		if db.Table(table).Count(&n).Error != nil {
			t.Fatal("reservation count unavailable")
		}
		return n
	}
	quantities := func(location string) (string, string) {
		var stocked, reserved string
		if pool.QueryRowContext(ctx, "SELECT stocked_quantity,reserved_quantity FROM inventory_levels WHERE inventory_item_id='fixture-stock' AND location_id=? AND deleted_at IS NULL", location).Scan(&stocked, &reserved) != nil {
			t.Fatal("reservation quantity unavailable")
		}
		return stocked, reserved
	}
	first := command("reserve-first", "1.000000000000000000", false)
	if _, err := run(first, true); !errors.Is(err, inventory.ErrSchema) {
		t.Fatal("disabled reservation capability accepted")
	}
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	beforeCount := count("inventory_reservations")
	if _, err := run(first, false); err != nil {
		t.Fatal("reservation staging failed")
	}
	if count("inventory_reservations") != beforeCount || count("inventory_reservation_operations") != 0 {
		t.Fatal("reservation rollback leaked rows")
	}
	if _, reserved := quantities("reserve-location"); reserved != "0.000000000000000000" {
		t.Fatal("reservation rollback leaked quantity")
	}
	// Distinct operations compete for the LAST unit, serialized by the level.
	var wg sync.WaitGroup
	type attempt struct {
		cmd    inventory.ReservationCommand
		result inventory.ReservationResult
		err    error
	}
	results := make(chan attempt, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := command(fmt.Sprintf("reserve-last-%d", i), "1.000000000000000000", false)
			result, err := run(cmd, true)
			results <- attempt{cmd, result, err}
		}(i)
	}
	wg.Wait()
	close(results)
	wins, insufficient := 0, 0
	var winner inventory.ReservationCommand
	for outcome := range results {
		if outcome.err == nil {
			wins++
			winner = outcome.cmd
		} else if errors.Is(outcome.err, inventory.ErrInsufficientStock) {
			insufficient++
		} else {
			t.Fatal("reservation concurrency dependency failure")
		}
	}
	if wins != 1 || insufficient != 7 || count("inventory_reservations") != beforeCount+1 || count("inventory_reservation_operations") != 1 {
		t.Fatal("last unit oversold")
	}
	if stocked, reserved := quantities("reserve-location"); stocked != "1.000000000000000000" || reserved != "1.000000000000000000" {
		t.Fatal("reservation changed stock instead of reserve")
	}
	if result, err := run(winner, true); err != nil || !result.Replayed {
		t.Fatal("reservation replay consumed twice")
	}
	// Replay must observe a committed receipt through an OLD RR snapshot.
	oldTx, err := pool.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal("old snapshot setup failed")
	}
	defer oldTx.Rollback()
	var oldN int
	if oldTx.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory_reservation_operations").Scan(&oldN) != nil {
		t.Fatal("old snapshot query failed")
	}
	exact := command("reserve-exact", "0.000000000000000001", true)
	if _, err := run(exact, true); err != nil {
		t.Fatal("backorder exact reservation failed")
	}
	if result, err := inventory.ApplyReservations(ctx, oldTx, exact); err != nil || !result.Replayed {
		t.Fatal("old snapshot hid current receipt")
	}
	if oldTx.Commit() != nil {
		t.Fatal("old snapshot replay commit failed")
	}
	if _, reserved := quantities("reserve-location"); reserved != "1.000000000000000001" {
		t.Fatal("reservation precision lost")
	}
	// A duplicated operation with altered owner or quantity is never accepted.
	changed := exact
	changed.OwnerID = "other-order"
	if _, err := run(changed, true); !errors.Is(err, inventory.ErrReservationConflict) {
		t.Fatal("reservation owner replay changed")
	}
	changed = exact
	changed.Reserve = append([]inventory.ReservationInput(nil), exact.Reserve...)
	changed.Reserve[0].Quantity = "0.000000000000000002"
	if _, err := run(changed, true); !errors.Is(err, inventory.ErrReservationConflict) {
		t.Fatal("reservation changed payload accepted")
	}
	// Legacy nullable backorder remains NULL in the release audit.
	exec("UPDATE inventory_reservations SET allow_backorder=NULL WHERE id='reserve-exact'")
	// Releasing is allowed while held, but does not clear or repair the hold.
	exec(`INSERT INTO inventory_migration_holds(inventory_item_id,location_id,reason,snapshot_sha256,evidence_sha256,evidence_json,policy_version) VALUES ('fixture-stock','reserve-location','negative_balance',REPEAT('a',32),REPEAT('b',32),'{}',1)`)
	if _, err := run(command("reserve-held", "0.000000000000000001", true), true); !errors.Is(err, inventory.ErrStockHeld) {
		t.Fatal("backorder bypassed historical hold")
	}
	release := inventory.ReservationCommand{OperationID: uuid.NewString(), OwnerID: "fixture-order", ActorRef: "fixture-owner-workflow", Kind: "release", Release: []inventory.ReservationBinding{{ID: winner.Reserve[0].ID, InventoryItemID: "fixture-stock", LocationID: "reserve-location", LineItemID: "fixture-order-line"}, {ID: "reserve-exact", InventoryItemID: "fixture-stock", LocationID: "reserve-location", LineItemID: "fixture-order-line"}}}
	wrong := release
	wrong.Release = append([]inventory.ReservationBinding(nil), release.Release...)
	wrong.Release[0].LineItemID = "another-line"
	if _, err := run(wrong, true); !errors.Is(err, inventory.ErrReservationConflict) {
		t.Fatal("cross-line release accepted")
	}
	if _, err := run(release, false); err != nil {
		t.Fatal("release rollback staging failed")
	}
	if _, reserved := quantities("reserve-location"); reserved != "1.000000000000000001" {
		t.Fatal("release rollback changed reserved")
	}
	if _, err := run(release, true); err != nil {
		t.Fatal("held reservation release failed")
	}
	var beforeJSON string
	if pool.QueryRowContext(ctx, "SELECT before_json FROM inventory_reservation_operations WHERE operation_id=?", release.OperationID).Scan(&beforeJSON) != nil {
		t.Fatal("release audit unavailable")
	}
	var releasedEvidence struct {
		Reservations []struct {
			ID             string
			AllowBackorder *bool `json:"allow_backorder"`
		}
	}
	if json.Unmarshal([]byte(beforeJSON), &releasedEvidence) != nil {
		t.Fatal("release audit invalid")
	}
	nullPreserved := false
	for _, r := range releasedEvidence.Reservations {
		if r.ID == "reserve-exact" && r.AllowBackorder == nil {
			nullPreserved = true
		}
	}
	if !nullPreserved {
		t.Fatal("legacy null backorder lost in audit")
	}
	if result, err := run(release, true); err != nil || !result.Replayed {
		t.Fatal("release replay decremented twice")
	}
	if _, reserved := quantities("reserve-location"); reserved != "0.000000000000000000" {
		t.Fatal("release did not restore reserved")
	}
	if count("inventory_migration_holds") != 1 {
		t.Fatal("release cleared historical hold")
	}
	otherRelease := release
	otherRelease.OperationID = uuid.NewString()
	if _, err := run(otherRelease, true); !errors.Is(err, inventory.ErrReservationConflict) {
		t.Fatal("different release decremented deleted rows twice")
	}
	// Shared item and multi-line demand is aggregated at a single location.
	batch := command("reserve-batch-a", "0.750000000000000000", false)
	batch.Reserve[0].LocationID = "reserve-other"
	second := batch.Reserve[0]
	second.ID = "reserve-batch-b"
	second.LineItemID = "another-line"
	batch.Reserve = append(batch.Reserve, second)
	if _, err := run(batch, true); !errors.Is(err, inventory.ErrInsufficientStock) {
		t.Fatal("shared item batch oversold stock")
	}
	if _, reserved := quantities("reserve-other"); reserved != "0.000000000000000000" {
		t.Fatal("rejected batch left reservation")
	}
	// Failure AFTER inventory updates, including the audit insert, must roll back
	// both inventory and the owning transaction's subsequent work.
	single := command("reserve-audit-fail", "0.500000000000000000", false)
	single.Reserve[0].LocationID = "reserve-other"
	exec(`CREATE TRIGGER reject_reservation_receipt BEFORE INSERT ON inventory_reservation_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial receipt failure'`)
	n := count("inventory_reservation_operations")
	if _, err := run(single, true); err == nil {
		t.Fatal("reservation receipt failure accepted")
	}
	if _, reserved := quantities("reserve-other"); reserved != "0.000000000000000000" || count("inventory_reservation_operations") != n {
		t.Fatal("receipt failure committed partial reservation")
	}
	exec("DROP TRIGGER reject_reservation_receipt")
	// Eight exact deliveries produce ONE row and ONE receipt.
	replayResults := make(chan attempt, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); res, err := run(single, true); replayResults <- attempt{single, res, err} }()
	}
	wg.Wait()
	close(replayResults)
	replayed := 0
	for outcome := range replayResults {
		if outcome.err != nil {
			t.Fatal("duplicate reservation dependency failure")
		}
		if outcome.result.Replayed {
			replayed++
		}
	}
	if replayed != 7 || count("inventory_reservation_operations") != n+1 {
		t.Fatal("duplicate reservation created multiple receipts")
	}
	if _, reserved := quantities("reserve-other"); reserved != "0.500000000000000000" {
		t.Fatal("duplicate reservation changed quantity twice")
	}
	// Snapshot imports and runtime reservations maintain independent facts.
	mismatch := command("reserve-mismatch", "1.000000000000000000", true)
	mismatch.Reserve[0].LocationID = "fixture-location"
	if _, err := run(mismatch, true); !errors.Is(err, inventory.ErrStockHeld) {
		t.Fatal("live reservation mismatch bypassed")
	}
	// An overflow and a colliding external reservation ID cannot leave a level write.
	collision := single
	collision.OperationID = uuid.NewString()
	if _, err := run(collision, true); !errors.Is(err, inventory.ErrReservationConflict) {
		t.Fatal("existing reservation identity overwritten")
	}
	overflow := command("reserve-overflow", "99999999999999999999.999999999999999999", true)
	overflow.Reserve[0].LocationID = "reserve-other"
	if _, err := run(overflow, true); !errors.Is(err, inventory.ErrStockHeld) {
		t.Fatal("reservation sum overflow accepted")
	}
	if _, reserved := quantities("reserve-other"); reserved != "0.500000000000000000" {
		t.Fatal("rejected write changed reserved quantity")
	}

	// Two commands list the same two locations in opposite order. Stable level
	// lock order prevents partial allocation and avoids an artificial deadlock.
	exec(`INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at) VALUES ('reserve-left','fixture-stock-unlinked','reserve-location',1,0,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('reserve-right','fixture-stock-unlinked','reserve-other',1,0,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	cross := make(chan attempt, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := command(fmt.Sprintf("cross-%d-a", i), "0.750000000000000000", false)
			cmd.Reserve[0].InventoryItemID = "fixture-stock-unlinked"
			other := cmd.Reserve[0]
			other.ID = fmt.Sprintf("cross-%d-b", i)
			other.LocationID = "reserve-other"
			cmd.Reserve = append(cmd.Reserve, other)
			if i == 1 {
				cmd.Reserve[0], cmd.Reserve[1] = cmd.Reserve[1], cmd.Reserve[0]
			}
			res, err := run(cmd, true)
			cross <- attempt{cmd, res, err}
		}(i)
	}
	wg.Wait()
	close(cross)
	crossWins, crossFailed := 0, 0
	for outcome := range cross {
		if outcome.err == nil {
			crossWins++
		} else if errors.Is(outcome.err, inventory.ErrInsufficientStock) {
			crossFailed++
		} else {
			t.Fatal("stable multi-location lock failed")
		}
	}
	if crossWins != 1 || crossFailed != 1 {
		t.Fatal("multi-location reservation partial allocation")
	}
	var min, max string
	if pool.QueryRowContext(ctx, "SELECT MIN(reserved_quantity),MAX(reserved_quantity) FROM inventory_levels WHERE inventory_item_id='fixture-stock-unlinked' AND deleted_at IS NULL").Scan(&min, &max) != nil || min != "0.750000000000000000" || max != min {
		t.Fatal("multi-location reservation left inconsistent levels")
	}
	// The owner can stage inventory, subsequently fail its own work, and roll back
	// every inventory row and receipt. No nested commit is allowed.
	ownerRollback := command("owner-rollback", "0.250000000000000000", false)
	ownerRollback.Reserve[0].LocationID = "reserve-other"
	ownerTx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal("owner transaction unavailable")
	}
	if _, err = inventory.ApplyReservations(ctx, ownerTx, ownerRollback); err != nil {
		ownerTx.Rollback()
		t.Fatal("owner transaction staging failed")
	}
	if _, err = ownerTx.ExecContext(ctx, "INSERT INTO deliberate_missing_owner_table(id) VALUES (1)"); err == nil {
		ownerTx.Rollback()
		t.Fatal("owner failure fixture did not fail")
	}
	if ownerTx.Rollback() != nil {
		t.Fatal("owner rollback failed")
	}
	if _, reserved := quantities("reserve-other"); reserved != "0.500000000000000000" {
		t.Fatal("owner failure committed nested inventory")
	}
	// Capability and exact model drift fail closed, even for a replay.
	exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.reservations'")
	if _, err := run(single, true); !errors.Is(err, inventory.ErrSchema) {
		t.Fatal("disabled reservation command replay accepted")
	}
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	exec("UPDATE inventory_runtime_schema SET model_sha256=REPEAT('0',64) WHERE id=1")
	if _, err := run(single, true); !errors.Is(err, inventory.ErrSchema) {
		t.Fatal("reservation model drift accepted")
	}
	t.Log(`RESERVATION_TRANSACTION_REHEARSAL {"migrations":44,"last_unit_attempts":8,"last_unit_wins":1,"insufficient_stock":7,"exact_duplicate_attempts":8,"exact_duplicate_replays":7,"old_repeatable_read_snapshot_replay":true,"rollback_atomic":true,"receipt_failure_rollback":true,"exact_backorder_quantity":true,"held_creation_rejected":true,"held_release_preserves_hold":true,"double_release_rejected":true,"cross_line_release_rejected":true,"shared_item_batch_aggregated":true,"live_mismatch_rejected":true,"overflow_rejected":true,"capability_disabled_by_default":true,"schema_drift_rejected":true,"stable_multi_location_locks":true,"owner_failure_rollback":true,"legacy_null_backorder_preserved":true,"http_route_exposed":false,"order_workflow_integrated":false}`)
}
