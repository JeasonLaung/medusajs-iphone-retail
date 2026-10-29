package identity_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedOrderNumbering(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned MySQL fixture required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime, cfg.Loc = true, time.UTC
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("owned numbering connection")
	}
	defer root.Close()
	if _, err = root.ExecContext(ctx, "CREATE DATABASE order_numbering_rehearsal"); err != nil {
		t.Fatal("owned numbering database")
	}
	cfg.DBName = "order_numbering_rehearsal"
	pool, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("owned numbering pool")
	}
	defer pool.Close()
	pool.SetMaxOpenConns(8)
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("owned numbering adapter")
	}
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := schema.Apply(ctx, pool, plan); err != nil || result.Applied != 60 {
		t.Fatal("numbering migrations", err)
	}
	txRun := func(fn func(*gorm.DB) error) error {
		return db.WithContext(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	}
	seed := orders.OrderNumberSeed{LastDisplayID: 700, LastTick: 179328960012345, EvidenceSHA256: sha256.Sum256([]byte("reviewed artificial source sequence and clock"))}
	seedRun := func(in orders.OrderNumberSeed) error {
		return txRun(func(tx *gorm.DB) error { return orders.SeedOrderNumbers(ctx, tx, in) })
	}
	allocate := func(tx *gorm.DB) (orders.OrderNumbers, error) { return orders.AllocateOrderNumbers(ctx, tx) }
	if _, err = orders.AllocateOrderNumbers(ctx, db); err != orders.ErrNumbering {
		t.Fatal("ordinary pool accepted")
	}
	if err = txRun(func(tx *gorm.DB) error { _, err := allocate(tx); return err }); err != orders.ErrNumbering {
		t.Fatal("unseeded allocation accepted")
	}
	insert := func(tx *gorm.DB, id string, n orders.OrderNumbers, deleted bool) error {
		var deletedAt any
		if deleted {
			deletedAt = gorm.Expr("UTC_TIMESTAMP(6)")
		}
		return tx.Table("orders").Create(map[string]any{"id": id, "display_id": n.DisplayID, "custom_display_id": n.CustomDisplayID, "version": 1, "status": "pending", "is_draft_order": false, "currency_code": "eur", "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)"), "deleted_at": deletedAt}).Error
	}
	if err = insert(db, "historical-deleted", orders.OrderNumbers{DisplayID: 650, CustomDisplayID: "2026103000000012345"}, true); err != nil {
		t.Fatal("owned history")
	}
	bad := seed
	bad.LastDisplayID = 649
	if seedRun(bad) != orders.ErrNumberSeed {
		t.Fatal("deleted display number ignored")
	}
	bad = seed
	bad.LastTick--
	if seedRun(bad) != orders.ErrNumberSeed {
		t.Fatal("deleted custom number ignored")
	}
	rollback := errors.New("owned late order failure")
	if err = txRun(func(tx *gorm.DB) error {
		if err := orders.SeedOrderNumbers(ctx, tx, seed); err != nil {
			return err
		}
		return rollback
	}); err != rollback {
		t.Fatal("seed rollback setup")
	}
	if err = txRun(func(tx *gorm.DB) error { _, err := allocate(tx); return err }); err != orders.ErrNumbering {
		t.Fatal("rolled back seed retained")
	}
	if err = seedRun(seed); err != nil {
		t.Fatal("reviewed seed failed", err)
	}
	bad = seed
	bad.EvidenceSHA256 = sha256.Sum256([]byte("different source"))
	if seedRun(bad) != orders.ErrNumberSeed {
		t.Fatal("different seed evidence accepted")
	}
	bad = seed
	bad.LastDisplayID++
	if seedRun(bad) != orders.ErrNumberSeed {
		t.Fatal("reseed changed counters")
	}
	// A far-future source clock exercises max(last+1, now) and wall-clock rollback.
	future := int64(200000000000000)
	if db.Exec("UPDATE order_number_allocator SET last_tick=? WHERE id=1", future).Error != nil {
		t.Fatal("owned clock fixture")
	}
	var rolled orders.OrderNumbers
	if err = txRun(func(tx *gorm.DB) error {
		var err error
		rolled, err = allocate(tx)
		if err != nil {
			return err
		}
		if err = insert(tx, "rolled-order", rolled, false); err != nil {
			return err
		}
		return rollback
	}); err != rollback {
		t.Fatal("order rollback")
	}
	if rolled.DisplayID != 701 || rolled.CustomDisplayID != "2033051811332000001" {
		t.Fatalf("clock continuation: %+v", rolled)
	}
	var group sync.WaitGroup
	for worker := 0; worker < 6; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for i := 0; i < 10; i++ {
				if err := txRun(func(tx *gorm.DB) error {
					n, err := allocate(tx)
					if err != nil {
						return err
					}
					return insert(tx, fmt.Sprintf("new-%d-%d", worker, i), n, false)
				}); err != nil {
					t.Errorf("concurrent numbering: %v", err)
					return
				}
			}
		}(worker)
	}
	group.Wait()
	var count, distinctDisplay, distinctCustom int64
	if db.Raw("SELECT COUNT(*),COUNT(DISTINCT display_id),COUNT(DISTINCT custom_display_id) FROM orders WHERE deleted_at IS NULL").Row().Scan(&count, &distinctDisplay, &distinctCustom) != nil || count != 60 || distinctDisplay != 60 || distinctCustom != 60 {
		t.Fatal("concurrent order numbers not unique")
	}
	var low, high int64
	if db.Raw("SELECT MIN(display_id),MAX(display_id) FROM orders WHERE deleted_at IS NULL").Row().Scan(&low, &high) != nil || low != 701 || high != 760 {
		t.Fatal("rolled back allocator consumed a committed identity")
	}
	if err = seedRun(seed); err != nil {
		t.Fatal("seed retry after allocation", err)
	}
	var state struct{ LastDisplayID, LastTick int64 }
	if db.Table("order_number_allocator").Select("last_display_id,last_tick").Where("id=1").Take(&state).Error != nil || state.LastDisplayID != 760 || state.LastTick != future+60 {
		t.Fatal("seed retry reset counters")
	}
	// Allocator SQL failure cannot return numbers or persist an order.
	if db.Exec("CREATE TRIGGER fail_order_numbers BEFORE UPDATE ON order_number_allocator FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned numbering failure'").Error != nil {
		t.Fatal("failure injection")
	}
	if err = txRun(func(tx *gorm.DB) error {
		n, err := allocate(tx)
		if n != (orders.OrderNumbers{}) {
			t.Error("failed allocation returned identity")
		}
		return err
	}); err != orders.ErrNumbering {
		t.Fatal("allocator SQL failure hidden")
	}
	if db.Exec("DROP TRIGGER fail_order_numbers").Error != nil {
		t.Fatal("failure cleanup")
	}
	if db.Exec("UPDATE order_number_allocator SET last_display_id=2147483647 WHERE id=1").Error != nil {
		t.Fatal("overflow setup")
	}
	if err = txRun(func(tx *gorm.DB) error { _, err := allocate(tx); return err }); err != orders.ErrNumbering {
		t.Fatal("integer order display overflow")
	}
	if db.Exec("DELETE FROM order_number_allocator WHERE id=1").Error != nil {
		t.Fatal("missing singleton setup")
	}
	if err = txRun(func(tx *gorm.DB) error { _, err := allocate(tx); return err }); err != orders.ErrNumbering {
		t.Fatal("missing singleton silently reinitialized")
	}
	t.Log(`ORDER_NUMBERING_REHEARSAL {"real_mysql":true,"explicit_seed_required":true,"deleted_history_checked":true,"exact_seed_retry_no_reset":true,"seed_rollback":true,"order_rollback":true,"concurrent_workers":6,"committed_orders":60,"unique_display_and_custom":true,"clock_rollback_monotonic":true,"sql_failure_closed":true,"overflow_closed":true,"missing_singleton_closed":true,"no_provider_or_customer_data":true}`)
}
