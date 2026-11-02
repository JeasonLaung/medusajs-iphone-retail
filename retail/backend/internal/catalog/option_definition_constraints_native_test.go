package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"strings"
	"testing"
	"time"
)

// Exercise migration-installed database enforcement using direct SQL writers.
func verifyDefinitionConstraints(t *testing.T, db *gorm.DB) {
	t.Helper()
	var triggers, audits int64
	if e := db.Raw("SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema=DATABASE() AND trigger_name IN ('catalog_option_title_insert_unique','catalog_option_title_update_unique','catalog_option_value_insert_unique','catalog_option_value_update_unique')").Scan(&triggers).Error; e != nil || triggers != 4 {
		t.Fatal("migration triggers absent", triggers, e)
	}
	if e := db.Table("catalog_option_uniqueness_audit").Where("duplicate_pairs=0").Count(&audits).Error; e != nil || audits != 4 {
		t.Fatal("migration audit incomplete", audits, e)
	}

	now := "UTC_TIMESTAMP(6)"
	insert := func(id, title string) error {
		return db.Exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES(?,?,FALSE,"+now+","+now+")", id, title).Error
	}
	must := func(query string, args ...any) {
		t.Helper()
		if e := db.Exec(query, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	violation := func(e error) {
		t.Helper()
		var failure *driver.MySQLError
		if !errors.As(e, &failure) || failure.Number != 1644 || failure.Message != "option full value uniqueness violation" {
			t.Fatalf("expected exact uniqueness signal, got %v", e)
		}
	}
	rejected := func(query string, args ...any) { t.Helper(); violation(db.Exec(query, args...).Error) }

	prefix := strings.Repeat("同", 2000)
	title := prefix + "A"
	if e := insert("prototype-a", title); e != nil {
		t.Fatal("candidate insert", e)
	}
	violation(insert("prototype-b", title))
	if e := insert("prototype-b", prefix+"B"); e != nil {
		t.Fatal("full suffix conflated", e)
	}
	rejected("UPDATE catalog_options SET title=? WHERE id='prototype-b'", title)
	must("UPDATE catalog_options SET deleted_at=UTC_TIMESTAMP(6) WHERE id='prototype-a'")
	if e := insert("prototype-c", title); e != nil {
		t.Fatal("deleted title not reusable", e)
	}
	rejected("UPDATE catalog_options SET deleted_at=NULL WHERE id='prototype-a'")
	must("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('prototype-exclusive',?,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", title)
	rejected("UPDATE catalog_options SET is_exclusive=FALSE WHERE id='prototype-exclusive'")
	must("UPDATE catalog_options SET title=CONCAT(title,' ') WHERE id='prototype-exclusive'")
	must("UPDATE catalog_options SET is_exclusive=FALSE WHERE id='prototype-exclusive'")
	must("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('prototype-case-a','Case',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('prototype-case-b','case',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	rejected("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('prototype-multi-a','multi conflict',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('prototype-multi-b','multi conflict',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	var partial int64
	if e := db.Table("catalog_options").Where("id IN ('prototype-multi-a','prototype-multi-b')").Count(&partial).Error; e != nil || partial != 0 {
		t.Fatal("multi-row conflict left partial insert", e)
	}
	value := func(id string, option any, text string) error {
		return db.Exec("INSERT INTO catalog_option_values(id,option_id,value,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, option, text).Error
	}
	if e := value("prototype-va", "prototype-b", title); e != nil {
		t.Fatal(e)
	}
	violation(value("prototype-vb", "prototype-b", title))
	if e := value("prototype-vb", "prototype-b", prefix+"B"); e != nil {
		t.Fatal(e)
	}
	if e := value("prototype-vc", "prototype-c", title); e != nil {
		t.Fatal("different owner conflated", e)
	}
	rejected("UPDATE catalog_option_values SET option_id='prototype-b' WHERE id='prototype-vc'")
	rejected("UPDATE catalog_option_values SET value=? WHERE id='prototype-vb'", title)
	must("UPDATE catalog_option_values SET deleted_at=UTC_TIMESTAMP(6) WHERE id='prototype-va'")
	if e := value("prototype-vd", "prototype-b", title); e != nil {
		t.Fatal(e)
	}
	rejected("UPDATE catalog_option_values SET deleted_at=NULL WHERE id='prototype-va'")
	for i := 0; i < 2; i++ {
		if e := value(fmt.Sprintf("prototype-null-%d", i), nil, title); e != nil {
			t.Fatal("NULL owner must remain distinct", e)
		}
	}
	// Repeatable-read stale snapshots must not defeat the trigger's current read.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, _ := db.DB()
	stale, e := pool.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if e != nil {
		t.Fatal(e)
	}
	defer stale.Rollback()
	var count int
	if e = stale.QueryRowContext(ctx, "SELECT COUNT(*) FROM catalog_options WHERE title='stale concurrent'").Scan(&count); e != nil || count != 0 {
		t.Fatal(e)
	}
	if e = insert("prototype-stale-first", "stale concurrent"); e != nil {
		t.Fatal(e)
	}
	_, e = stale.ExecContext(ctx, "INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('prototype-stale-second','stale concurrent',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	violation(e)
	if e = stale.Rollback(); e != nil {
		t.Fatal(e)
	}
	// Two independent transactions bypass every Go command/mutex. One is held
	// inside SQL until the actual data_lock_waits row proves trigger contention.
	tx1, e := pool.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx1.Rollback()
	tx2, e := pool.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx2.Rollback()
	var connection uint64
	if e = tx2.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&connection); e != nil {
		t.Fatal(e)
	}
	if _, e = tx1.ExecContext(ctx, "INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('prototype-race-first','race concurrent',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))"); e != nil {
		t.Fatal(e)
	}
	result := make(chan error, 1)
	go func() {
		_, err := tx2.ExecContext(ctx, "INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('prototype-race-second','race concurrent',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
		result <- err
	}()
	waited := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waits int
		err := pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.threads t ON t.THREAD_ID=w.REQUESTING_THREAD_ID WHERE t.PROCESSLIST_ID=?`, connection).Scan(&waits)
		if err != nil {
			t.Fatal(err)
		}
		if waits > 0 {
			waited = true
			break
		}
		select {
		case err := <-result:
			t.Fatal("concurrent trigger did not wait", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waited {
		t.Fatal("trigger lock contention not observed")
	}
	if e = tx1.Commit(); e != nil {
		t.Fatal(e)
	}
	violation(<-result)
	if e = tx2.Rollback(); e != nil {
		t.Fatal(e)
	}

	// The value trigger must serialize independent writers too, not just titles.
	firstValue, e := pool.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer firstValue.Rollback()
	secondValue, e := pool.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer secondValue.Rollback()
	if e = secondValue.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&connection); e != nil {
		t.Fatal(e)
	}
	if _, e = firstValue.ExecContext(ctx, "INSERT INTO catalog_option_values(id,option_id,value,created_at,updated_at) VALUES('prototype-race-va','prototype-b','race value',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))"); e != nil {
		t.Fatal(e)
	}
	go func() {
		_, err := secondValue.ExecContext(ctx, "INSERT INTO catalog_option_values(id,option_id,value,created_at,updated_at) VALUES('prototype-race-vb','prototype-b','race value',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
		result <- err
	}()
	waited = false
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waits int
		err := pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.threads t ON t.THREAD_ID=w.REQUESTING_THREAD_ID WHERE t.PROCESSLIST_ID=?`, connection).Scan(&waits)
		if err != nil {
			t.Fatal(err)
		}
		if waits > 0 {
			waited = true
			break
		}
		select {
		case err := <-result:
			t.Fatal("concurrent value trigger did not wait", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waited {
		t.Fatal("value trigger lock contention not observed")
	}
	if e = firstValue.Commit(); e != nil {
		t.Fatal(e)
	}
	violation(<-result)
	if e = secondValue.Rollback(); e != nil {
		t.Fatal(e)
	}
	var titleRows, valueRows int64
	if e = db.Table("catalog_options").Where("title='race concurrent'").Count(&titleRows).Error; e != nil || titleRows != 1 {
		t.Fatal("title race final state", titleRows, e)
	}
	if e = db.Table("catalog_option_values").Where("option_id='prototype-b' AND value='race value'").Count(&valueRows).Error; e != nil || valueRows != 1 {
		t.Fatal("value race final state", valueRows, e)
	}
	must("DELETE FROM catalog_option_values WHERE id LIKE 'prototype-%'")
	must("DELETE FROM catalog_options WHERE id LIKE 'prototype-%'")
	proof := map[string]any{"trigger_candidate_only": false, "migration_installed": true, "database_full_text_comparison": true, "same_long_prefix_distinct_suffix": true, "direct_sql_duplicates_rejected": true, "soft_delete_reuse_restore_conflict": true, "exclusive_to_global_conflict": true, "case_and_trailing_space_distinct": true, "multi_row_atomic_rejection": true, "value_owner_change_conflict": true, "null_owner_duplicate_allowed": true, "repeatable_read_stale_snapshot_rejected": true, "actual_trigger_lock_wait_observed": true, "concurrent_duplicate_rejected": true, "value_trigger_lock_wait_observed": true, "concurrent_value_duplicate_rejected": true, "race_title_rows": titleRows, "race_value_rows": valueRows}
	encoded, _ := json.Marshal(proof)
	t.Log("OPTION_FULL_UNIQUE_NATIVE " + string(encoded))
}
