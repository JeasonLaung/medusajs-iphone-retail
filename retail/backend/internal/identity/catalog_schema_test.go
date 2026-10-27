package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"regexp"
	"testing"
	"testing/fstest"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedCatalogSchemaTransition(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned SQL fixture required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("schema fixture SQL unavailable")
	}
	defer root.Close()
	cfg.DBName = "schema_catalog_fresh"
	openDB := func(name string) *sql.DB {
		// Names below are fixed test constants, never source data or user input.
		if _, err := root.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
			t.Fatal("schema fixture database unavailable")
		}
		local := *cfg
		local.DBName = name
		db, err := sql.Open("mysql", local.FormatDSN())
		if err != nil {
			t.Fatal("schema fixture connection unavailable")
		}
		t.Cleanup(func() { db.Close() })
		return db
	}
	source, _ := fs.Sub(migrations.Files, ".")
	allFiles := fstest.MapFS{}
	allEntries, _ := fs.ReadDir(source, ".")
	for _, e := range allEntries {
		if e.Name() < "0011" {
			b, _ := fs.ReadFile(source, e.Name())
			allFiles[e.Name()] = &fstest.MapFile{Data: b}
		}
	}
	all, err := schema.Load(allFiles)
	if err != nil {
		t.Fatal("complete schema plan invalid")
	}
	older := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal("schema files unavailable")
	}
	for _, entry := range entries {
		if entry.Name() < "0010" {
			data, err := migrations.Files.ReadFile(entry.Name())
			if err != nil {
				t.Fatal("older schema file unavailable")
			}
			older[entry.Name()] = &fstest.MapFile{Data: data}
		}
	}
	previous, err := schema.Load(older)
	if err != nil {
		t.Fatal("older schema plan invalid")
	}
	fresh := openDB("schema_catalog_fresh")
	result, err := schema.Apply(ctx, fresh, all)
	if err != nil || result.Applied != 10 {
		t.Fatal("fresh catalog schema migration failed")
	}
	if schema.CheckApplied(ctx, fresh, all) != nil {
		t.Fatal("complete migration readiness failed")
	}
	ddl, err := migrations.Files.ReadFile("0010_catalog_relational.sql")
	if err != nil {
		t.Fatal("catalog schema definition unavailable")
	}
	tables := regexp.MustCompile("CREATE TABLE `([a-z_]+)`").FindAllSubmatch(ddl, -1)
	if len(tables) != 45 {
		t.Fatal("catalog schema table count differs")
	}
	columns := 0
	for _, table := range tables {
		var count int
		if fresh.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND extra NOT LIKE '%GENERATED%'", string(table[1])).Scan(&count) != nil || count == 0 {
			t.Fatal("catalog schema table missing")
		}
		columns += count
	}
	if columns != 374 {
		t.Fatal("catalog source column count differs")
	}
	result, err = schema.Apply(ctx, fresh, all)
	if err != nil || result.Skipped != 10 {
		t.Fatal("catalog migration replay failed")
	}
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: fresh, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	if err != nil || catalog.Ready(ctx, gdb) != nil {
		t.Fatal("catalog marker readiness failed")
	}
	var model string
	if fresh.QueryRowContext(ctx, "SELECT model_sha256 FROM catalog_runtime_schema WHERE id=1").Scan(&model) != nil {
		t.Fatal("catalog marker missing")
	}
	if _, err = fresh.ExecContext(ctx, "UPDATE catalog_runtime_schema SET model_sha256=REPEAT('0',64) WHERE id=1"); err != nil || catalog.Ready(ctx, gdb) == nil {
		t.Fatal("altered model marker accepted")
	}
	if _, err = fresh.ExecContext(ctx, "UPDATE catalog_runtime_schema SET model_sha256=? WHERE id=1", model); err != nil {
		t.Fatal("model marker fixture restore failed")
	}
	if _, err = fresh.ExecContext(ctx, "UPDATE catalog_runtime_schema SET writes_enabled=TRUE WHERE id=1"); err == nil {
		t.Fatal("unimplemented write capability enabled")
	}
	if _, err = fresh.ExecContext(ctx, "UPDATE schema_migration_steps SET inflight=TRUE WHERE version='0010_catalog_relational.sql'"); err != nil || schema.CheckApplied(ctx, fresh, all) == nil {
		t.Fatal("inflight migration accepted by readiness")
	}
	if _, err = fresh.ExecContext(ctx, "UPDATE schema_migration_steps SET inflight=FALSE WHERE version='0010_catalog_relational.sql'"); err != nil {
		t.Fatal("journal fixture restore failed")
	}
	if _, err = fresh.ExecContext(ctx, "UPDATE schema_migrations SET checksum=REPEAT('0',64) WHERE version='0010_catalog_relational.sql'"); err != nil || schema.CheckApplied(ctx, fresh, all) == nil {
		t.Fatal("checksum drift accepted by readiness")
	}
	upgrade := openDB("schema_catalog_upgrade")
	result, err = schema.Apply(ctx, upgrade, previous)
	if err != nil || result.Applied != 9 {
		t.Fatal("empty upgrade starting plan failed")
	}
	if _, err = upgrade.ExecContext(ctx, `INSERT INTO admin_users(id,email,password_hash,name,role,enabled,created_at,updated_at,auth_version) VALUES('new-local-fixture','new-local@example.invalid','artificial-hash','Retained new user','admin',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),3)`); err != nil {
		t.Fatal("new local identity fixture failed")
	}
	result, err = schema.Apply(ctx, upgrade, all)
	if err != nil || result.Applied != 1 || result.Skipped != 9 || schema.CheckApplied(ctx, upgrade, all) != nil {
		t.Fatal("empty draft upgrade failed")
	}
	var retained int
	if upgrade.QueryRowContext(ctx, "SELECT COUNT(*) FROM admin_users WHERE id='new-local-fixture' AND auth_version=3 AND password_hash='artificial-hash'").Scan(&retained) != nil || retained != 1 {
		t.Fatal("upgrade altered new account")
	}
	for _, sample := range []struct{ name, seed string }{
		{"schema_catalog_product", `INSERT INTO catalog_products(id,handle,title,status,payload,created_at,updated_at) VALUES('legacy-fixture','legacy-fixture','Preserve me','draft','{}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`},
		{"schema_catalog_region", `INSERT INTO commerce_regions(id,name,currency_code,payload) VALUES('legacy-fixture','Preserve me','eur','{}')`},
		{"schema_catalog_collision", `CREATE TABLE catalog_images(marker INT NOT NULL);`},
	} {
		db := openDB(sample.name)
		r, err := schema.Apply(ctx, db, previous)
		if err != nil || r.Applied != 9 {
			t.Fatal("older schema fixture failed")
		}
		if schema.CheckApplied(ctx, db, all) == nil {
			t.Fatal("old plan accepted by current readiness")
		}
		if _, err = db.ExecContext(ctx, sample.seed); err != nil {
			t.Fatal("transition precondition fixture failed")
		}
		_, err = schema.Apply(ctx, db, all)
		if !errors.Is(err, schema.ErrUncertain) {
			t.Fatal("nonempty or conflicting schema transition accepted")
		}
		_, err = schema.Apply(ctx, db, all)
		if !errors.Is(err, schema.ErrUncertain) {
			t.Fatal("failed transition retried without reconciliation")
		}
		var backups int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name LIKE 'legacy_0001_%'").Scan(&backups) != nil || backups != 0 {
			t.Fatal("failed precondition renamed draft tables")
		}
		var rows int
		if sample.name == "schema_catalog_product" {
			err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM catalog_products WHERE id='legacy-fixture' AND title='Preserve me'").Scan(&rows)
		} else if sample.name == "schema_catalog_region" {
			err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM commerce_regions WHERE id='legacy-fixture' AND name='Preserve me'").Scan(&rows)
		} else {
			err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='catalog_images' AND column_name='marker'").Scan(&rows)
		}
		if err != nil || rows != 1 {
			t.Fatal("failed transition changed prior data or structure")
		}
	}
	counts := map[string]int{"explicit_migrations": 10, "catalog_tables": 45, "source_columns": 374, "fresh_and_replay": 1, "empty_draft_upgrade_preserved_new_account": 1, "nonempty_draft_refusals": 2, "target_collision_refusals": 1, "failed_precondition_preserved_tables": 3, "readiness_rejections": 6, "write_enable_rejected": 1}
	encoded, _ := json.Marshal(counts)
	t.Log("CATALOG_SCHEMA_REHEARSAL " + string(encoded))
}
