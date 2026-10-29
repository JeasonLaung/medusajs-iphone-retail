package identity_test

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedGeoSchema(t *testing.T) {
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
		t.Fatal("owned geo server unavailable")
	}
	defer root.Close()
	open := func(name string) (*sql.DB, *gorm.DB) {
		t.Helper()
		if _, err := root.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
			t.Fatal("owned geo database unavailable")
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
			t.Fatal("geo SQL adapter unavailable")
		}
		return db, gdb
	}
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	db, gdb := open("geo_schema_fresh")
	result, err := schema.Apply(ctx, db, plan)
	if err != nil || result.Applied != 64 || schema.CheckApplied(ctx, db, plan) != nil || fulfillment.GeoReady(ctx, gdb) != nil {
		t.Fatal("fresh geo schema unavailable")
	}
	if result, err = schema.Apply(ctx, db, plan); err != nil || result.Skipped != 64 {
		t.Fatal("geo replay changed schema")
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if gdb.Exec(query, args...).Error != nil {
			t.Fatal("artificial geo SQL fixture failed")
		}
	}
	reject := func(query string) {
		t.Helper()
		if gdb.Exec(query).Error == nil {
			t.Fatal("invalid geo configuration accepted")
		}
	}
	exec(`INSERT INTO fulfillment_sets(id,name,type,created_at,updated_at) VALUES('geo-set','Set','shipping',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_service_zones(id,name,fulfillment_set_id,created_at,updated_at) VALUES('geo-zone','Zone','geo-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	postal := `{ "eq" : ["00123", "12345"], "number":9007199254740993 }`
	metadata := `{"label":"区域", "label":"duplicate preserved"}`
	exec(`INSERT INTO fulfillment_geo_zones(id,type,country_code,province_code,city,service_zone_id,postal_expression,metadata,created_at,updated_at) VALUES('geo','zip','DK','01','Århus','geo-zone',?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, postal, metadata)
	var row struct{ CountryCode, ProvinceCode, City, PostalExpression, Metadata string }
	if gdb.Table("fulfillment_geo_zones").Where("id='geo'").Take(&row).Error != nil || row.CountryCode != "DK" || row.ProvinceCode != "01" || row.City != "Århus" || row.PostalExpression != postal || row.Metadata != metadata {
		t.Fatal("geo exact text/JSON roundtrip differs")
	}
	exec(`INSERT INTO fulfillment_geo_zones(id,type,country_code,service_zone_id,created_at,updated_at) VALUES('nullable','country','dk','geo-zone',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	var nulls int
	if db.QueryRowContext(ctx, `SELECT province_code IS NULL AND city IS NULL AND postal_expression IS NULL AND metadata IS NULL AND deleted_at IS NULL FROM fulfillment_geo_zones WHERE id='nullable'`).Scan(&nulls) != nil || nulls != 1 {
		t.Fatal("SQL null changed")
	}
	reject(`UPDATE fulfillment_geo_zones SET type='continent' WHERE id='geo'`)
	reject(`UPDATE fulfillment_geo_zones SET service_zone_id='missing' WHERE id='geo'`)
	reject(`UPDATE fulfillment_geo_zones SET postal_expression='invalid JSON' WHERE id='geo'`)
	reject(`UPDATE fulfillment_geo_zones SET metadata='invalid JSON' WHERE id='geo'`)
	reject(`UPDATE geo_runtime_schema SET writes_enabled=TRUE`)
	exec(`UPDATE geo_runtime_schema SET reads_enabled=FALSE`)
	if fulfillment.GeoReady(ctx, gdb) != fulfillment.ErrGeoSchema {
		t.Fatal("disabled geo model accepted")
	}
	exec(`UPDATE geo_runtime_schema SET reads_enabled=TRUE,model_sha256=REPEAT('0',64)`)
	if fulfillment.GeoReady(ctx, gdb) != fulfillment.ErrGeoSchema || fulfillment.GeoReady(ctx, nil) != fulfillment.ErrGeoSchema {
		t.Fatal("geo model drift accepted")
	}
	exec(`DELETE FROM fulfillment_service_zones WHERE id='geo-zone'`)
	var count int64
	if gdb.Table("fulfillment_geo_zones").Count(&count).Error != nil || count != 0 {
		t.Fatal("geo parent cascade differs")
	}
	previous := fstest.MapFS{}
	entries, _ := fs.ReadDir(migrations.Files, ".")
	for _, entry := range entries {
		if entry.Name() < "0030" {
			data, _ := fs.ReadFile(migrations.Files, entry.Name())
			previous[entry.Name()] = &fstest.MapFile{Data: data}
		}
	}
	old, err := schema.Load(previous)
	if err != nil {
		t.Fatal(err)
	}
	blocked, _ := open("geo_schema_existing")
	if _, err := schema.Apply(ctx, blocked, old); err != nil {
		t.Fatal(err)
	}
	if _, err := blocked.ExecContext(ctx, `CREATE TABLE fulfillment_geo_zones(id INT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := blocked.ExecContext(ctx, `INSERT INTO fulfillment_geo_zones VALUES(17)`); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Apply(ctx, blocked, plan); err == nil {
		t.Fatal("existing geo table overwritten")
	}
	var kept int
	if blocked.QueryRowContext(ctx, `SELECT id FROM fulfillment_geo_zones`).Scan(&kept) != nil || kept != 17 {
		t.Fatal("existing geo row lost")
	}
	if blocked.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='geo_runtime_schema'`).Scan(&kept) != nil || kept != 0 {
		t.Fatal("geo guard created partial domain")
	}
	t.Log(`GEO_SCHEMA_REHEARSAL {"migrations":61,"tables":1,"source_columns":11,"exact_json_text_and_leading_zero":true,"sql_null_distinct":true,"enum_json_and_parent_fk_rejected":true,"parent_cascade":true,"replay_unchanged":true,"existing_tables_preserved":true,"schema_drift_rejected":true,"writes_enabled":false,"address_matching_integrated":false}`)
}
