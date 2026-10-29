package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/promotion"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedPromotionUsage(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned SQL fixture required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime, cfg.Loc = true, time.UTC
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("owned promotion server unavailable")
	}
	defer root.Close()
	open := func(name string) (*sql.DB, *gorm.DB) {
		t.Helper()
		if _, err := root.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
			t.Fatal("owned promotion database unavailable")
		}
		c := *cfg
		c.DBName = name
		db, err := sql.Open("mysql", c.FormatDSN())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
		if err != nil {
			t.Fatal("promotion SQL adapter unavailable")
		}
		return db, gdb
	}
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	db, gdb := open("promotion_usage_fresh")
	result, err := schema.Apply(ctx, db, plan)
	if err != nil || result.Applied != 31 || schema.CheckApplied(ctx, db, plan) != nil || promotion.Ready(ctx, gdb) != nil {
		t.Fatal("fresh promotion schema unavailable")
	}
	if result, err = schema.Apply(ctx, db, plan); err != nil || result.Skipped != 31 {
		t.Fatal("promotion replay changed schema")
	}

	exec := func(query string, args ...any) {
		t.Helper()
		if err := gdb.Exec(query, args...).Error; err != nil {
			t.Fatalf("artificial promotion fixture: %v", err)
		}
	}
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	setup := func(suffix, kind, limit, used string) {
		exec("INSERT INTO promotion_campaigns(id,name,campaign_identifier,starts_at,ends_at,created_at,updated_at) VALUES(?,?,?,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "c_"+suffix, "Campaign", "CAMPAIGN_"+suffix)
		exec("INSERT INTO promotions(id,code,campaign_id,is_automatic,type,status,is_tax_inclusive,`limit`,used,created_at,updated_at) VALUES(?,?,?,FALSE,'standard','active',FALSE,NULL,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "p_"+suffix, "CODE_"+suffix, "c_"+suffix)
		exec("INSERT INTO promotion_campaign_budgets(id,type,campaign_id,`limit`,raw_limit,used,raw_used,currency_code,attribute,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "b_"+suffix, kind, "c_"+suffix, limit, `{"value":"`+limit+`","precision":100}`, used, `{"value":"`+used+`","precision":100}`, "eur", "customer_id")
	}
	call := func(actions []promotion.UsageAction, attributes map[string]string) error {
		return gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, err := promotion.RegisterUsage(ctx, tx, actions, promotion.UsageRegistration{Currency: "eur", Attributes: attributes, At: now})
			return err
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	}
	used := func(table, id string) string {
		t.Helper()
		var raw string
		if db.QueryRowContext(ctx, "SELECT raw_used FROM "+table+" WHERE id=?", id).Scan(&raw) != nil {
			t.Fatal("usage missing")
		}
		var v struct{ Value string }
		if json.Unmarshal([]byte(raw), &v) != nil {
			t.Fatal("raw usage malformed")
		}
		return v.Value
	}
	setup("spend", "spend", "100", "0")
	actions := []promotion.UsageAction{{PromotionID: "p_spend", Amount: "60"}}
	if _, err := promotion.RegisterUsage(ctx, gdb, actions, promotion.UsageRegistration{Currency: "eur", At: now}); err != promotion.ErrUsageInput {
		t.Fatal("autocommit budget registration accepted")
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; results <- call(actions, nil) }()
	}
	close(start)
	wg.Wait()
	close(results)
	wins, limits := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, promotion.ErrUsageLimit) {
			limits++
		} else {
			t.Fatalf("unexpected registration error: %v", err)
		}
	}
	if wins != 1 || limits != 7 || used("promotion_campaign_budgets", "b_spend") != "60" {
		t.Fatal("concurrent spend overspent or lost usage")
	}
	// A consistent read from before another commit must not govern locked usage.
	stale := gdb.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	var n string
	if stale.Raw("SELECT CAST(used AS CHAR) FROM promotion_campaign_budgets WHERE id='b_spend'").Scan(&n).Error != nil {
		t.Fatal("stale snapshot setup failed")
	}
	if err := call([]promotion.UsageAction{{PromotionID: "p_spend", Amount: "30"}}, nil); err != nil {
		t.Fatal(err)
	}
	_, err = promotion.RegisterUsage(ctx, stale, []promotion.UsageAction{{PromotionID: "p_spend", Amount: "20"}}, promotion.UsageRegistration{Currency: "eur", At: now})
	stale.Rollback()
	if err != promotion.ErrUsageLimit || used("promotion_campaign_budgets", "b_spend") != "90" {
		t.Fatal("old RR snapshot bypassed current budget")
	}
	// Duplicate actions for one promotion consume usage once and aggregate spend.
	setup("usage", "usage", "2", "0")
	exec("UPDATE promotions SET `limit`=2 WHERE id='p_usage'")
	if err := call([]promotion.UsageAction{{PromotionID: "p_usage", Amount: "1"}, {PromotionID: "p_usage", Amount: "2"}}, nil); err != nil {
		t.Fatal(err)
	}
	if used("promotion_campaign_budgets", "b_usage") != "1" {
		t.Fatal("duplicate action consumed multiple usage counts")
	}
	var count int
	if db.QueryRowContext(ctx, "SELECT used FROM promotions WHERE id='p_usage'").Scan(&count) != nil || count != 1 {
		t.Fatal("promotion count duplicated")
	}
	// Two promotion codes sharing one activity each consume the shared counter.
	exec("INSERT INTO promotions(id,code,campaign_id,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES('p_shared','SHARED','c_usage',FALSE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	if err := call([]promotion.UsageAction{{PromotionID: "p_usage", Amount: "1"}, {PromotionID: "p_shared", Amount: "1"}}, nil); err != promotion.ErrUsageLimit || used("promotion_campaign_budgets", "b_usage") != "1" {
		t.Fatal("shared activity lost a distinct promotion count")
	}
	// Attribute budget first-row creation is serialized on its parent budget.
	setup("attribute", "use_by_attribute", "1", "0")
	attribute := strings.Repeat("a", 512) + "-x"
	start = make(chan struct{})
	results = make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- call([]promotion.UsageAction{{PromotionID: "p_attribute", Amount: "1"}}, map[string]string{"customer_id": attribute})
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	wins, limits = 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, promotion.ErrUsageLimit) {
			limits++
		} else {
			t.Fatalf("attribute registration error: %v", err)
		}
	}
	if wins != 1 || limits != 7 {
		t.Fatal("attribute first-create race")
	}
	if err := call([]promotion.UsageAction{{PromotionID: "p_attribute", Amount: "1"}}, map[string]string{"customer_id": attribute + " "}); err != nil {
		t.Fatal("complete attribute values collapsed")
	}
	if err := call([]promotion.UsageAction{{PromotionID: "p_attribute", Amount: "1"}}, map[string]string{"customer_id": strings.ToUpper(attribute)}); err != nil {
		t.Fatal("attribute case collapsed")
	}
	if used("promotion_campaign_budgets", "b_attribute") != "3" {
		t.Fatal("attribute global counter changed")
	}
	if err := call([]promotion.UsageAction{{PromotionID: "p_attribute", Amount: "1"}}, nil); err != promotion.ErrUsageInput {
		t.Fatal("missing attribute bypassed budget")
	}
	// Unsupported budget types remain disabled even though schema preserves them.
	setup("unsupported", "spend_by_attribute", "1", "0")
	if err := call([]promotion.UsageAction{{PromotionID: "p_unsupported", Amount: "1"}}, map[string]string{"customer_id": "fixture"}); err != promotion.ErrUsageUnavailable {
		t.Fatal("unsupported budget silently accepted")
	}
	// Exact raw beyond scale18 is added without rounding or float conversion.
	setup("exact", "spend", "2", "0.123456789012345678")
	exec(`UPDATE promotion_campaign_budgets SET raw_used='{"value":"0.123456789012345678901","precision":100}' WHERE id='b_exact'`)
	if err := call([]promotion.UsageAction{{PromotionID: "p_exact", Amount: "0.000000000000000000001"}}, nil); err != nil || used("promotion_campaign_budgets", "b_exact") != "0.123456789012345678902" {
		t.Fatal("precise raw budget lost digits")
	}
	// Deliberate failure after successful phase must revert every counter.
	late := errors.New("late artificial phase failure")
	err = gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := promotion.RegisterUsage(ctx, tx, []promotion.UsageAction{{PromotionID: "p_usage", Amount: "1"}, {PromotionID: "p_exact", Amount: "0.1"}}, promotion.UsageRegistration{Currency: "eur", At: now}); e != nil {
			return e
		}
		return late
	})
	if err != late || used("promotion_campaign_budgets", "b_usage") != "1" || used("promotion_campaign_budgets", "b_exact") != "0.123456789012345678902" {
		t.Fatal("late failure left partial usage")
	}
	// Database fault AFTER promotion counts change rolls back those writes too.
	exec("CREATE TRIGGER fixture_budget_failure BEFORE UPDATE ON promotion_campaign_budgets FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='fixture failure'")
	if err := call([]promotion.UsageAction{{PromotionID: "p_usage", Amount: "1"}}, nil); err != promotion.ErrUsageStorage {
		t.Fatal("injected budget failure not propagated")
	}
	exec("DROP TRIGGER fixture_budget_failure")
	if db.QueryRowContext(ctx, "SELECT used FROM promotions WHERE id='p_usage'").Scan(&count) != nil || count != 1 || used("promotion_campaign_budgets", "b_usage") != "1" {
		t.Fatal("failed budget write retained promotion count")
	}
	// Start is inclusive; end is exclusive, evaluated using one trusted clock.
	setup("time", "usage", "3", "0")
	exec("UPDATE promotion_campaigns SET starts_at=?,ends_at=? WHERE id='c_time'", now, now.Add(time.Hour))
	if err := call([]promotion.UsageAction{{PromotionID: "p_time", Amount: "1"}}, nil); err != nil {
		t.Fatal("campaign start rejected")
	}
	exec("UPDATE promotion_campaigns SET ends_at=? WHERE id='c_time'", now)
	if err := call([]promotion.UsageAction{{PromotionID: "p_time", Amount: "1"}}, nil); err != promotion.ErrUsageUnavailable {
		t.Fatal("exclusive campaign end accepted")
	}
	exec("UPDATE promotion_campaign_budgets SET currency_code='usd' WHERE id='b_exact'")
	if err := call([]promotion.UsageAction{{PromotionID: "p_exact", Amount: "1"}}, nil); err != promotion.ErrUsageUnavailable {
		t.Fatal("budget currency mismatch accepted")
	}
	exec("UPDATE promotions SET status='inactive' WHERE id='p_exact'")
	if err := call([]promotion.UsageAction{{PromotionID: "p_exact", Amount: "1"}}, nil); err == nil {
		t.Fatal("inactive coupon consumed")
	}
	// A duplicate full-value attribute is a configuration conflict, never guessed.
	var existingID string
	if db.QueryRowContext(ctx, "SELECT id FROM promotion_budget_usages WHERE BINARY attribute_value=BINARY ? LIMIT 1", attribute).Scan(&existingID) != nil {
		t.Fatal("attribute fixture missing")
	}
	exec("INSERT INTO promotion_budget_usages(id,attribute_value,used,budget_id,raw_used,created_at,updated_at) SELECT 'dup_usage',attribute_value,used,budget_id,raw_used,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) FROM promotion_budget_usages WHERE id=?", existingID)
	if err := call([]promotion.UsageAction{{PromotionID: "p_attribute", Amount: "1"}}, map[string]string{"customer_id": attribute}); err != promotion.ErrUsageUnavailable {
		t.Fatal("ambiguous attribute used")
	}

	revert := func(actions []promotion.UsageAction, attributes map[string]string) error {
		return gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, e := promotion.RevertUsage(ctx, tx, actions, promotion.UsageRegistration{Currency: "eur", Attributes: attributes, At: now})
			return e
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	}
	setup("revert", "spend", "100", "20")
	exec("UPDATE promotions SET `limit`=1,used=1 WHERE id='p_revert'")
	if err := revert([]promotion.UsageAction{{PromotionID: "p_revert", Amount: "3"}, {PromotionID: "p_revert", Amount: "4"}}, nil); err != nil || used("promotion_campaign_budgets", "b_revert") != "13" {
		t.Fatal("revert duplicate actions did not aggregate spend")
	}
	if db.QueryRowContext(ctx, "SELECT used FROM promotions WHERE id='p_revert'").Scan(&count) != nil || count != 0 {
		t.Fatal("at-limit promotion could not be reversed once")
	}
	if err := revert([]promotion.UsageAction{{PromotionID: "p_revert", Amount: "20"}}, nil); err != nil || used("promotion_campaign_budgets", "b_revert") != "0" {
		t.Fatal("revert did not clamp at zero")
	}
	// Distinct promotion codes on one activity release distinct usage counts.
	setup("reverseusage", "usage", "10", "3")
	exec("INSERT INTO promotions(id,code,campaign_id,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES('p_reverseother','REVERSEOTHER','c_reverseusage',FALSE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	if err := revert([]promotion.UsageAction{{PromotionID: "p_reverseusage", Amount: "1"}, {PromotionID: "p_reverseother", Amount: "2"}}, nil); err != nil || used("promotion_campaign_budgets", "b_reverseusage") != "1" {
		t.Fatal("shared usage reversal lost a code")
	}
	// Register and reverse contend on the same parent locks, with no lost update.
	setup("mix", "spend", "1000", "100")
	start = make(chan struct{})
	results = make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			actions := []promotion.UsageAction{{PromotionID: "p_mix", Amount: "10"}}
			if index%2 == 0 {
				results <- call(actions, nil)
			} else {
				results <- revert(actions, nil)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("mixed register/reverse: %v", err)
		}
	}
	if used("promotion_campaign_budgets", "b_mix") != "100" {
		t.Fatal("mixed registration and reversal lost a committed delta")
	}
	// Attribute exhaustion preserves old evidence by soft deletion; recreation
	// uses a fresh row, and an older RR snapshot must see the active successor.
	setup("reverseattr", "use_by_attribute", "2", "0")
	attr := map[string]string{"customer_id": "reverse-fixture"}
	attrActions := []promotion.UsageAction{{PromotionID: "p_reverseattr", Amount: "1"}}
	if err := call(attrActions, attr); err != nil {
		t.Fatal(err)
	}
	if err := call(attrActions, attr); err != nil {
		t.Fatal(err)
	}
	var originalUsage string
	if db.QueryRowContext(ctx, "SELECT id FROM promotion_budget_usages WHERE budget_id='b_reverseattr' AND deleted_at IS NULL").Scan(&originalUsage) != nil {
		t.Fatal("attribute evidence missing")
	}
	if err := revert(attrActions, attr); err != nil || used("promotion_budget_usages", originalUsage) != "1" {
		t.Fatal("attribute decrement failed")
	}
	if err := revert(attrActions, attr); err != nil {
		t.Fatal(err)
	}
	var deleted bool
	if db.QueryRowContext(ctx, "SELECT deleted_at IS NOT NULL FROM promotion_budget_usages WHERE id=?", originalUsage).Scan(&deleted) != nil || !deleted {
		t.Fatal("attribute history erased or still active")
	}
	if err := revert(attrActions, attr); err != nil || used("promotion_campaign_budgets", "b_reverseattr") != "0" {
		t.Fatal("missing attribute reversal created usage")
	}
	if err := call(attrActions, attr); err != nil {
		t.Fatal(err)
	}
	var successor string
	if db.QueryRowContext(ctx, "SELECT id FROM promotion_budget_usages WHERE budget_id='b_reverseattr' AND deleted_at IS NULL").Scan(&successor) != nil || successor == originalUsage {
		t.Fatal("deleted attribute history reused")
	}
	stale = gdb.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if stale.Raw("SELECT CAST(used AS CHAR) FROM promotion_budget_usages WHERE id=?", successor).Scan(&n).Error != nil {
		t.Fatal("attribute snapshot read failed")
	}
	if err := call(attrActions, attr); err != nil {
		t.Fatal(err)
	}
	if _, err := promotion.RevertUsage(ctx, stale, attrActions, promotion.UsageRegistration{Currency: "eur", Attributes: attr, At: now}); err != nil {
		stale.Rollback()
		t.Fatal(err)
	}
	if stale.Commit().Error != nil || used("promotion_budget_usages", successor) != "1" {
		t.Fatal("attribute reverse used stale RR snapshot")
	}
	// Precision remains exact in both directions.
	setup("reverseexact", "spend", "1", "0.123456789012345678")
	exec(`UPDATE promotion_campaign_budgets SET raw_used='{"value":"0.123456789012345678902","precision":100}' WHERE id='b_reverseexact'`)
	if err := revert([]promotion.UsageAction{{PromotionID: "p_reverseexact", Amount: "0.000000000000000000001"}}, nil); err != nil || used("promotion_campaign_budgets", "b_reverseexact") != "0.123456789012345678901" {
		t.Fatal("reversal rounded exact raw")
	}
	// Caller failure after reversal restores both history and counters.
	err = gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := promotion.RevertUsage(ctx, tx, attrActions, promotion.UsageRegistration{Currency: "eur", Attributes: attr, At: now}); e != nil {
			return e
		}
		return late
	})
	if err != late || used("promotion_campaign_budgets", "b_reverseattr") != "1" || used("promotion_budget_usages", successor) != "1" {
		t.Fatal("late reversal failure was not atomic")
	}
	exec("CREATE TRIGGER fixture_reverse_failure BEFORE UPDATE ON promotion_campaign_budgets FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='reverse failure'")
	if err := revert(attrActions, attr); err != promotion.ErrUsageStorage {
		t.Fatal("reverse write fault not surfaced")
	}
	exec("DROP TRIGGER fixture_reverse_failure")
	if db.QueryRowContext(ctx, "SELECT deleted_at IS NOT NULL FROM promotion_budget_usages WHERE id=?", successor).Scan(&deleted) != nil || deleted || used("promotion_campaign_budgets", "b_reverseattr") != "1" {
		t.Fatal("reverse fault left soft delete committed")
	}
	if _, err := promotion.RevertUsage(ctx, gdb, attrActions, promotion.UsageRegistration{Currency: "eur", Attributes: attr, At: now}); err != promotion.ErrUsageInput {
		t.Fatal("autocommit reversal accepted")
	}
	t.Log(`PROMOTION_USAGE_REHEARSAL {"migrations":31,"concurrent_attempts":8,"spend_wins":1,"attribute_first_create_wins":1,"current_locking_read_after_old_snapshot":true,"duplicate_actions_once_per_promotion":true,"shared_budget_counts_distinct_promotions":true,"complete_attribute_case_and_trailing_space":true,"missing_attribute_rejected":true,"unsupported_budget_rejected":true,"exact_raw_beyond_scale18":true,"late_failure_rollback":true,"write_fault_rollback":true,"campaign_start_inclusive_end_exclusive":true,"currency_mismatch_rejected":true,"autocommit_rejected":true,"revert_phase_integrated":true,"revert_shared_budget_and_clamp":true,"mixed_register_revert_concurrent":16,"attribute_soft_delete_recreate_history":true,"attribute_revert_current_read":true,"revert_precise_raw":true,"revert_late_and_write_fault_rollback":true,"expired_reversal_requires_saved_ledger":true,"durable_idempotency_integrated":false}`)
}
