package identity_test

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/eurorefurb/retail/internal/promotion"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedPromotionSchema(t *testing.T) {
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
	db, gdb := open("promotion_schema_fresh")
	result, err := schema.Apply(ctx, db, plan)
	if err != nil || result.Applied != 79 || schema.CheckApplied(ctx, db, plan) != nil || promotion.Ready(ctx, gdb) != nil {
		t.Fatal("fresh promotion schema unavailable")
	}
	if result, err = schema.Apply(ctx, db, plan); err != nil || result.Skipped != 79 {
		t.Fatal("promotion replay changed schema")
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if gdb.Exec(query, args...).Error != nil {
			t.Fatal("artificial promotion SQL fixture failed")
		}
	}
	reject := func(query string) {
		t.Helper()
		if gdb.Exec(query).Error == nil {
			t.Fatal("invalid promotion configuration accepted")
		}
	}

	exec(`INSERT INTO promotion_campaigns(id,name,campaign_identifier,created_at,updated_at) VALUES('campaign','Campaign','complete long identifier',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec("INSERT INTO promotions(id,code,campaign_id,is_automatic,type,status,is_tax_inclusive,`limit`,used,metadata,created_at,updated_at) VALUES('promo','SAVE','campaign',FALSE,'standard','active',TRUE,NULL,0,'null',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec(`INSERT INTO promotion_application_methods(id,value,raw_value,max_quantity,apply_to_quantity,buy_rules_min_quantity,type,target_type,allocation,promotion_id,created_at,updated_at) VALUES('method',0.123456789012345678,'{ "value":"0.123456789012345678901", "precision":30 }',NULL,NULL,NULL,'fixed','items','each','promo',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	raw := `{ "value":"9007199254740993.123456789012345678901", "precision":40 }`
	exec("INSERT INTO promotion_campaign_budgets(id,type,campaign_id,`limit`,raw_limit,used,raw_used,created_at,updated_at) VALUES('budget','use_by_attribute','campaign',NULL,NULL,9007199254740993.123456789012345678,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", raw)
	attribute := "fixture-attribute-" + strings.Repeat("x", 512)
	exec(`INSERT INTO promotion_budget_usages(id,attribute_value,used,budget_id,raw_used,created_at,updated_at) VALUES('usage',?,0,'budget','{"value":"0","precision":20}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, attribute)
	exec("INSERT INTO promotion_rules(id,attribute,operator,created_at,updated_at) VALUES('rule','items.product_id','in',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec(`INSERT INTO promotion_rule_values(id,promotion_rule_id,value,created_at,updated_at) VALUES('value','rule','00123',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO promotion_buy_rules VALUES('method','rule')`)
	exec(`INSERT INTO promotion_target_rules VALUES('method','rule')`)
	exec(`INSERT INTO promotion_order_rules VALUES('promo','rule')`)
	var saved, scalar, gotAttribute, ruleValue string
	if db.QueryRowContext(ctx, "SELECT raw_used,CAST(used AS CHAR) FROM promotion_campaign_budgets WHERE id='budget'").Scan(&saved, &scalar) != nil || saved != raw || scalar != "9007199254740993.123456789012345678" {
		t.Fatal("promotion exact scalar/raw changed")
	}
	if db.QueryRowContext(ctx, "SELECT attribute_value FROM promotion_budget_usages WHERE id='usage'").Scan(&gotAttribute) != nil || gotAttribute != attribute {
		t.Fatal("attribute text truncated")
	}
	if db.QueryRowContext(ctx, "SELECT value FROM promotion_rule_values WHERE id='value'").Scan(&ruleValue) != nil || ruleValue != "00123" {
		t.Fatal("rule value coerced")
	}
	verifyPromotionActiveRead(t, ctx, gdb)
	reject(`UPDATE promotions SET type='unknown'`)
	reject(`UPDATE promotions SET status='unknown'`)
	reject(`UPDATE promotions SET is_automatic=2`)
	reject(`UPDATE promotions SET metadata='invalid'`)
	reject(`UPDATE promotion_application_methods SET raw_value='invalid'`)
	reject(`UPDATE promotion_application_methods SET allocation='unknown'`)
	reject(`UPDATE promotion_application_methods SET target_type='unknown'`)
	reject(`UPDATE promotion_campaign_budgets SET type='unknown'`)
	reject(`UPDATE promotion_rules SET operator='nin'`)
	reject(`INSERT INTO promotion_target_rules VALUES('missing','rule')`)
	reject(`INSERT INTO promotion_buy_rules VALUES('method','missing')`)
	reject(`INSERT INTO promotion_order_rules VALUES('missing','rule')`)
	reject(`UPDATE promotion_budget_usages SET budget_id='missing'`)
	reject(`INSERT INTO promotion_target_rules VALUES('method','rule')`)
	reject(`UPDATE promotions SET campaign_id='missing'`)
	reject(`INSERT INTO promotions(id,code,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES('dup','SAVE',FALSE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`UPDATE promotions SET deleted_at=UTC_TIMESTAMP(6) WHERE id='promo'`)
	exec(`INSERT INTO promotions(id,code,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES('replacement','SAVE',FALSE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`UPDATE promotions SET deleted_at=NULL WHERE id='replacement'`)
	reject(`UPDATE promotions SET deleted_at=NULL WHERE id='promo'`)
	exec(`UPDATE promotion_campaign_budgets SET type='spend_by_attribute' WHERE id='budget'`)
	// Enum preservation does not enable this unsupported runtime budget type.
	exec(`DELETE FROM promotion_rules WHERE id='rule'`)
	for _, table := range []string{"promotion_rule_values", "promotion_order_rules", "promotion_buy_rules", "promotion_target_rules"} {
		var count int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count) != nil || count != 0 {
			t.Fatal("promotion rule cascade incomplete")
		}
	}
	exec(`UPDATE promotion_runtime_schema SET reads_enabled=FALSE`)
	if promotion.Ready(ctx, gdb) != promotion.ErrSchema {
		t.Fatal("disabled promotion model accepted")
	}
	exec(`UPDATE promotion_runtime_schema SET reads_enabled=TRUE,model_sha256=REPEAT('0',64)`)
	if promotion.Ready(ctx, gdb) != promotion.ErrSchema || promotion.Ready(ctx, nil) != promotion.ErrSchema {
		t.Fatal("promotion schema drift accepted")
	}
	reject(`UPDATE promotion_runtime_schema SET writes_enabled=TRUE`)
	previous := fstest.MapFS{}
	entries, _ := fs.ReadDir(migrations.Files, ".")
	for _, entry := range entries {
		if entry.Name() < "0031" {
			data, _ := fs.ReadFile(migrations.Files, entry.Name())
			previous[entry.Name()] = &fstest.MapFile{Data: data}
		}
	}
	old, err := schema.Load(previous)
	if err != nil {
		t.Fatal(err)
	}
	blocked, _ := open("promotion_schema_existing")
	if _, err := schema.Apply(ctx, blocked, old); err != nil {
		t.Fatal(err)
	}
	if _, err := blocked.ExecContext(ctx, `CREATE TABLE promotion_campaigns(id INT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := blocked.ExecContext(ctx, `INSERT INTO promotion_campaigns VALUES(17)`); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Apply(ctx, blocked, plan); err == nil {
		t.Fatal("existing promotion table overwritten")
	}
	var kept int
	if blocked.QueryRowContext(ctx, `SELECT id FROM promotion_campaigns`).Scan(&kept) != nil || kept != 17 {
		t.Fatal("existing promotion row lost")
	}
	if blocked.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='promotion_runtime_schema'`).Scan(&kept) != nil || kept != 0 {
		t.Fatal("promotion guard created partial domain")
	}
	t.Log(`PROMOTION_SCHEMA_REHEARSAL {"migrations":61,"tables":10,"source_columns":75,"exact_raw_and_scalar":true,"complete_attribute_text":true,"rule_leading_zero":true,"enum_json_boolean_fk_rejected":true,"active_code_uniqueness":true,"rule_link_cascade":true,"replay_unchanged":true,"existing_tables_preserved":true,"schema_drift_rejected":true,"writes_enabled":false,"budget_registration_integrated":false}`)
}
