package identity_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedCartRuntime(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned MySQL/Redis required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime, cfg.Loc = true, time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("owned cart root connection unavailable")
	}
	defer root.Close()
	if _, err := root.ExecContext(ctx, "CREATE DATABASE cart_runtime_rehearsal"); err != nil {
		t.Fatal("owned cart database unavailable")
	}
	cfg.DBName = "cart_runtime_rehearsal"
	gdb, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("owned cart connection unavailable")
	}
	db, _ := gdb.DB()
	defer db.Close()
	db.SetMaxOpenConns(12)
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("cart runtime migration plan unavailable")
	}
	r, err := schema.Apply(ctx, db, plan)
	if err != nil || r.Applied != 45 {
		t.Fatal("cart runtime migration failed")
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if gdb.WithContext(ctx).Exec(query, args...).Error != nil {
			t.Fatal("artificial cart fixture mutation failed")
		}
	}
	count := func(table string, expected int) {
		t.Helper()
		var actual int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&actual) != nil || actual != expected {
			t.Fatal("cart runtime evidence count differs")
		}
	}
	exec(`INSERT INTO commerce_sales_channels(id,name,is_disabled,created_at,updated_at) VALUES ('channel','Channel',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_stores(id,name,default_sales_channel_id,created_at,updated_at) VALUES ('shop','Shop','channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('other-shop','Other','channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_regions(id,name,currency_code,automatic_taxes,created_at,updated_at) VALUES ('region','Europe','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('usd-region','US','usd',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_store_currencies(id,store_id,currency_code,is_default,created_at,updated_at) VALUES ('currency','shop','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_store_locales(id,store_id,locale_code,created_at,updated_at) VALUES ('en','shop','en',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('zh','shop','zh-CN',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	in := cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("artificial authenticated scope")), StoreID: "shop", RegionID: "region", Locale: "zh-CN"}
	if result, err := cart.Create(ctx, gdb, in); err != cart.ErrSchema || result != (cart.CreateResult{}) {
		t.Fatal("disabled cart creation granted authority")
	}
	count("cart_ownership", 0)
	exec(`UPDATE cart_command_capabilities SET enabled=TRUE`)
	created, err := cart.Create(ctx, gdb, in)
	if err != nil || created.Replayed || created.Reference.CartID == "" || created.Reference.StoreID != "shop" || len(created.Reference.Generation) != 64 {
		t.Fatalf("new owned cart creation failed: %v", err)
	}
	for _, table := range []string{"carts", "cart_ownership", "cart_creation_operations", "cart_creation_scopes"} {
		count(table, 1)
	}
	count("cart_line_items", 0)
	count("admin_users", 0)
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", MaxRetries: -1, Protocol: 2, DisableIdentity: true, ContextTimeoutEnabled: true, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	defer redisClient.Close()
	sessions, err := sessionstore.New(redisClient, "eurorefurb:cart-runtime:v1:")
	if err != nil {
		t.Fatal("owned cart sessions unavailable")
	}
	issued, err := sessions.IssueCart(ctx, created.Reference)
	if err != nil {
		t.Fatal("committed cart possession not issued")
	}
	ref, err := sessions.AuthorizeCartWrite(ctx, issued.Token, issued.CSRF)
	if err != nil || ref != created.Reference {
		t.Fatal("committed cart possession not resolved")
	}
	snapshot, err := cart.Inspect(ctx, gdb, "shop", ref)
	if err != nil || snapshot.ID != ref.CartID || snapshot.RegionID != "region" || snapshot.CurrencyCode != "eur" || snapshot.Locale != "zh-CN" || snapshot.SalesChannelID != "channel" || snapshot.Version != "1" {
		t.Fatal("owned cart inspection failed")
	}
	serialized, _ := json.Marshal(snapshot)
	for _, private := range []string{"customer_id", "email", "metadata", "generation", "store_id"} {
		if strings.Contains(string(serialized), private) {
			t.Fatal("private cart binding escaped snapshot")
		}
	}
	replayed, err := cart.Create(ctx, gdb, in)
	if err != nil || !replayed.Replayed || replayed.Reference != ref {
		t.Fatal("creation replay changed cart")
	}
	changed := in
	changed.Locale = "en"
	if _, err := cart.Create(ctx, gdb, changed); err != cart.ErrConflict {
		t.Fatal("creation operation accepted changed parameters")
	}
	changed.ScopeHash = sha256.Sum256([]byte("other authenticated anonymous scope"))
	separate, err := cart.Create(ctx, gdb, changed)
	if err != nil || separate.Reference == ref || separate.Reference.CartID == ref.CartID || separate.Replayed {
		t.Fatal("separate scope recovered another cart")
	}
	// Concurrent creation of one operation must produce exactly one durable cart.
	race := in
	race.OperationID = uuid.NewString()
	const attempts = 8
	results := make(chan cart.CreateResult, attempts)
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for j := 0; j < attempts; j++ {
		wg.Go(func() { r, err := cart.Create(ctx, gdb, race); results <- r; errs <- err })
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent cart create failed")
		}
	}
	fresh, sameID := 0, ""
	for r := range results {
		if !r.Replayed {
			fresh++
		}
		if sameID != "" && sameID != r.Reference.CartID {
			t.Fatal("concurrent create duplicated cart")
		}
		sameID = r.Reference.CartID
	}
	if fresh != 1 {
		t.Fatal("concurrent create did not have one fresh result")
	}
	count("carts", 3)
	count("cart_ownership", 3)
	count("cart_creation_operations", 3)
	// Missing source runtime ownership is never backfilled, even for an open cart.
	exec(`INSERT INTO carts(id,currency_code,created_at,updated_at) VALUES ('legacy-cart','eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	forged := ref
	forged.CartID = "legacy-cart"
	if _, err := cart.Inspect(ctx, gdb, "shop", forged); err != cart.ErrDenied {
		t.Fatal("legacy cart ID granted ownership")
	}
	forged = ref
	forged.StoreID = "other-shop"
	if _, err := cart.Inspect(ctx, gdb, "other-shop", forged); err != cart.ErrDenied {
		t.Fatal("cart authority crossed configured store")
	}
	for _, change := range []string{"completed_at=UTC_TIMESTAMP(6)", "deleted_at=UTC_TIMESTAMP(6)", "customer_id='historical-customer'"} {
		exec("UPDATE carts SET "+change+" WHERE id=?", ref.CartID)
		if _, err := cart.Inspect(ctx, gdb, "shop", ref); err != cart.ErrDenied {
			t.Fatal("closed or customer-bound cart remained anonymous")
		}
		if _, err := cart.Create(ctx, gdb, in); err != cart.ErrDenied {
			t.Fatal("creation replay bypassed current lifecycle")
		}
		exec(`UPDATE carts SET completed_at=NULL,deleted_at=NULL,customer_id=NULL WHERE id=?`, ref.CartID)
	}
	exec(`UPDATE cart_ownership SET active=FALSE WHERE cart_id=?`, ref.CartID)
	if _, err := cart.Inspect(ctx, gdb, "shop", ref); err != cart.ErrDenied {
		t.Fatal("inactive ownership granted cart access")
	}
	exec(`UPDATE cart_ownership SET active=TRUE WHERE cart_id=?`, ref.CartID)
	for _, query := range []string{
		`UPDATE cart_ownership SET version=0 WHERE cart_id=?`,
		`UPDATE cart_ownership SET active=2 WHERE cart_id=?`,
		`UPDATE cart_ownership SET generation='invalid' WHERE cart_id=?`,
	} {
		if gdb.Exec(query, ref.CartID).Error == nil {
			t.Fatal("invalid durable cart authority accepted")
		}
	}
	exec(`UPDATE commerce_sales_channels SET is_disabled=TRUE WHERE id='channel'`)
	if _, err := cart.Inspect(ctx, gdb, "shop", ref); err != cart.ErrConfiguration {
		t.Fatal("disabled store channel granted cart access")
	}
	exec(`UPDATE commerce_sales_channels SET is_disabled=FALSE WHERE id='channel'`)
	exec(`UPDATE commerce_stores SET deleted_at=UTC_TIMESTAMP(6) WHERE id='shop'`)
	if _, err := cart.Create(ctx, gdb, in); err != cart.ErrConfiguration {
		t.Fatal("deleted store creation replay accepted")
	}
	exec(`UPDATE commerce_stores SET deleted_at=NULL WHERE id='shop'`)
	// Invalid region/currency/locale fails before any new scope/cart persists.
	for _, regionLocale := range [][2]string{{"missing", "en"}, {"usd-region", "en"}, {"region", "fr"}} {
		bad := in
		bad.OperationID = uuid.NewString()
		bad.ScopeHash = sha256.Sum256([]byte("uncommitted bad scope"))
		bad.RegionID, bad.Locale = regionLocale[0], regionLocale[1]
		if _, err := cart.Create(ctx, gdb, bad); err != cart.ErrRegion {
			t.Fatal("unavailable region or locale created cart")
		}
	}
	count("cart_creation_scopes", 2)
	// Late receipt failure rolls back cart, authority and new scope together.
	exec(`CREATE TRIGGER reject_cart_receipt BEFORE INSERT ON cart_creation_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial failure'`)
	failed := in
	failed.OperationID = uuid.NewString()
	failed.ScopeHash = sha256.Sum256([]byte("failed committed scope"))
	if result, err := cart.Create(ctx, gdb, failed); err != cart.ErrStorage || result != (cart.CreateResult{}) {
		t.Fatal("failed receipt returned cart authority")
	}
	exec(`DROP TRIGGER reject_cart_receipt`)
	count("carts", 4)
	count("cart_ownership", 3)
	count("cart_creation_operations", 3)
	count("cart_creation_scopes", 2)
	if _, err := cart.LockForWrite(ctx, gdb, "shop", ref, "1"); err != cart.ErrInput {
		t.Fatal("cart authorization accepted autocommit")
	}
	locked := gdb.WithContext(ctx).Begin()
	defer locked.Rollback()
	if locked.Error != nil {
		t.Fatal("owned cart lock transaction unavailable")
	}
	if _, err := cart.Create(ctx, locked, in); err != cart.ErrInput {
		t.Fatal("create returned before caller-owned commit")
	}
	if _, err := cart.LockForWrite(ctx, locked, "shop", ref, "1"); err != nil {
		t.Fatal("current write authority rejected")
	}
	started, finished := make(chan int64, 1), make(chan error, 1)
	go func() {
		finished <- gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var connection int64
			if tx.Raw("SELECT CONNECTION_ID()").Scan(&connection).Error != nil {
				return errors.New("artificial connection unavailable")
			}
			started <- connection
			_, err := cart.LockForWrite(ctx, tx, "shop", ref, "1")
			return err
		})
	}()
	connection := <-started
	deadline, observed := time.Now().Add(15*time.Second), false
	for time.Now().Before(deadline) {
		select {
		case err := <-finished:
			t.Fatalf("second cart writer ended before lock observation: %v", err)
		default:
		}
		var waiting int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.innodb_trx WHERE trx_mysql_thread_id=? AND trx_state='LOCK WAIT'", connection).Scan(&waiting) != nil {
			t.Fatal("owned cart lock observation failed")
		}
		if waiting == 1 {
			observed = true
			break
		}
		// INNODB_TRX is an engine snapshot refreshed after a quiet interval.
		// Tight polling can keep observing the pre-wait cached RUNNING state.
		time.Sleep(250 * time.Millisecond)
	}
	if !observed {
		t.Fatal("second cart writer did not actually wait on MySQL lock")
	}
	if locked.Exec(`UPDATE cart_ownership SET version=2 WHERE cart_id=?`, ref.CartID).Error != nil || locked.Commit().Error != nil {
		t.Fatal("owned cart version commit failed")
	}
	if err := <-finished; err != cart.ErrConflict {
		t.Fatal("waiting writer accepted stale version")
	}
	rolledBack := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := cart.LockForWrite(ctx, tx, "shop", ref, "2"); err != nil {
			return err
		}
		if tx.Exec(`UPDATE cart_ownership SET version=3 WHERE cart_id=?`, ref.CartID).Error != nil {
			return cart.ErrStorage
		}
		return errors.New("artificial later command failure")
	})
	if rolledBack == nil {
		t.Fatal("participant unexpectedly committed caller transaction")
	}
	if s, err := cart.Inspect(ctx, gdb, "shop", ref); err != nil || s.Version != "2" {
		t.Fatal("caller rollback changed version")
	}
	// Redis still resolves its old reference; durable generation wins.
	exec(`UPDATE cart_ownership SET generation=REPEAT('f',64),version=version+1 WHERE cart_id=?`, ref.CartID)
	stale, err := sessions.ResolveCart(ctx, issued.Token)
	if err != nil || stale != ref {
		t.Fatal("stale Redis reference fixture unavailable")
	}
	if _, err := cart.Inspect(ctx, gdb, "shop", stale); err != cart.ErrDenied {
		t.Fatal("Redis restored revoked durable authority")
	}
	if _, err := cart.Create(ctx, gdb, in); err != cart.ErrDenied {
		t.Fatal("creation replay reissued revoked generation")
	}
	if sessions.RevokeCart(ctx, issued.Token) != nil {
		t.Fatal("owned cart session cleanup failed")
	}
	checkPaymentEffects(t, ctx, gdb)
	checkPaymentRequests(t, ctx, gdb)
	checkCartAmounts(t, ctx, gdb, sessions)
	checkCartPaymentOperations(t, ctx, gdb)
	checkCartPaymentExecution(t, ctx, gdb)
	checkCartPaymentCompensation(t, ctx, gdb)
	checkCartPaymentCreationExecution(t, ctx, gdb)
	checkCartItems(t, ctx, gdb, sessions)
	checkCartContacts(t, ctx, gdb, sessions)
	checkCartHTTP(t, ctx, gdb, sessions, redisClient)
	exec(`UPDATE cart_command_capabilities SET enabled=FALSE WHERE command='cart.access'`)
	if _, err := cart.Inspect(ctx, gdb, "shop", separate.Reference); err != cart.ErrSchema {
		t.Fatal("disabled access capability accepted")
	}
	t.Log(`CART_RUNTIME_REHEARSAL {"migrations":45,"atomic_cart_authority_receipt":true,"anonymous_only":true,"real_redis_to_mysql_binding":true,"concurrent_creates":8,"fresh_create":1,"scoped_idempotency":true,"changed_request_rejected":true,"current_authority_on_replay":true,"legacy_id_denied":true,"cross_store_denied":true,"closed_deleted_customer_bound_denied":true,"region_currency_locale_validated":true,"receipt_failure_atomic":true,"autocommit_nested_create_rejected":true,"mysql_lock_wait_observed":true,"stale_version_rejected":true,"caller_rollback_preserved":true,"stale_redis_generation_denied":true,"disabled_capability_denied":true,"http_integrated":false,"checkout_integrated":false}`)
}
