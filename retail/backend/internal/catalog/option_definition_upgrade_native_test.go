package catalog

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
)

func verifyDefinitionUniquenessUpgrade(t *testing.T, control *sql.DB, cfg *driver.Config) {
	t.Helper()
	oldFS := fstest.MapFS{}
	names, e := fs.ReadDir(migrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range names {
		if !strings.HasSuffix(name.Name(), ".sql") || name.Name() >= "0097_" {
			continue
		}
		raw, e := migrations.Files.ReadFile(name.Name())
		if e != nil {
			t.Fatal(e)
		}
		oldFS[name.Name()] = &fstest.MapFile{Data: raw}
	}
	old, e := schema.Load(oldFS)
	if e != nil {
		t.Fatal(e)
	}
	fullFS := fstest.MapFS{}
	for name, entry := range oldFS {
		fullFS[name] = entry
	}
	raw97, e := migrations.Files.ReadFile("0097_catalog_option_full_uniqueness.sql")
	if e != nil {
		t.Fatal(e)
	}
	fullFS["0097_catalog_option_full_uniqueness.sql"] = &fstest.MapFile{Data: raw97}
	full, e := schema.Load(fullFS)
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"clean", "title", "value", "deleted"} {
		t.Run("upgrade_"+kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			name := cfg.DBName + "_" + kind
			if _, e := control.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); e != nil {
				t.Fatal(e)
			}
			defer func() {
				if _, e := control.Exec("DROP DATABASE `" + name + "`"); e != nil {
					t.Error(e)
				}
			}()
			copy := cfg.Clone()
			copy.DBName = name
			db, e := sql.Open("mysql", copy.FormatDSN())
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			if applied, e := schema.Apply(ctx, db, old); e != nil || applied.Applied != 96 {
				t.Fatal("old plan", applied, e)
			}
			exec := func(query string, args ...any) {
				t.Helper()
				if _, e := db.ExecContext(ctx, query, args...); e != nil {
					t.Fatal(e)
				}
			}
			long := strings.Repeat("同", 2000)
			exec("INSERT INTO catalog_options(id,title,is_exclusive,metadata,created_at,updated_at) VALUES('upgrade-a',?,FALSE,'{\"preserved\":9007199254740993123}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('upgrade-b',?,FALSE,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", long+"A", long+"B")
			exec("INSERT INTO catalog_option_values(id,option_id,value,created_at,updated_at) VALUES('upgrade-va','upgrade-a',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('upgrade-vb','upgrade-a',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", long+"A", long+"B")
			switch kind {
			case "title":
				exec("UPDATE catalog_options SET title=? WHERE id='upgrade-b'", long+"A")
			case "value":
				exec("UPDATE catalog_option_values SET value=? WHERE id='upgrade-vb'", long+"A")
			case "deleted":
				exec("UPDATE catalog_options SET deleted_at=UTC_TIMESTAMP(6),title=? WHERE id='upgrade-b'", long+"A")
				exec("UPDATE catalog_option_values SET deleted_at=UTC_TIMESTAMP(6),value=? WHERE id='upgrade-vb'", long+"A")
			}
			// Compare complete source facts, not a count or a prefix/hash identity.
			facts := func() string {
				t.Helper()
				var text string
				if e := db.QueryRowContext(ctx, `SELECT JSON_ARRAY(
     (SELECT JSON_ARRAYAGG(JSON_ARRAY(id,title,metadata,created_at,updated_at,deleted_at,is_exclusive)) FROM catalog_options),
     (SELECT JSON_ARRAYAGG(JSON_ARRAY(id,option_id,value,metadata,created_at,updated_at,deleted_at)) FROM catalog_option_values))`).Scan(&text); e != nil {
					t.Fatal(e)
				}
				return text
			}
			before := facts()
			applied, e := schema.Apply(ctx, db, full)
			blocked := kind == "title" || kind == "value"
			if blocked {
				if !errors.Is(e, schema.ErrUncertain) {
					t.Fatal("duplicate upgrade not explicitly uncertain", e)
				}
				if schema.CheckApplied(ctx, db, full) == nil {
					t.Fatal("duplicate upgrade marked ready")
				}
				var count, next int
				var status string
				var inflight bool
				if e := db.QueryRowContext(ctx, "SELECT statement_count,next_statement,inflight,status FROM schema_migration_steps WHERE version='0097_catalog_option_full_uniqueness.sql'").Scan(&count, &next, &inflight, &status); e != nil || count != 7 || next != 1 || !inflight || status != "failed" {
					t.Fatal("failed preflight journal", count, next, inflight, status, e)
				}
				var triggers int
				if e := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema=DATABASE()").Scan(&triggers); e != nil || triggers != 0 {
					t.Fatal("trigger installed before duplicate audit", triggers, e)
				}
				if _, e := schema.Apply(ctx, db, full); !errors.Is(e, schema.ErrUncertain) {
					t.Fatal("uncertain migration automatically retried", e)
				}
			} else {
				if e != nil || applied.Applied != 1 || applied.Skipped != 96 || schema.CheckApplied(ctx, db, full) != nil {
					t.Fatal("clean upgrade", applied, e)
				}
				var count, next int
				var status string
				var inflight bool
				if e := db.QueryRowContext(ctx, "SELECT statement_count,next_statement,inflight,status FROM schema_migration_steps WHERE version='0097_catalog_option_full_uniqueness.sql'").Scan(&count, &next, &inflight, &status); e != nil || count != 7 || next != 7 || inflight || status != "applied" {
					t.Fatal("compound migration journal", count, next, inflight, status, e)
				}
				if replay, e := schema.Apply(ctx, db, full); e != nil || replay.Applied != 0 || replay.Skipped != 97 {
					t.Fatal("migration replay", replay, e)
				}
			}
			if after := facts(); after != before {
				t.Fatal("migration changed source facts")
			}
		})
	}
	t.Log(`OPTION_UNIQUENESS_UPGRADE {"from":96,"to":97,"clean_and_soft_deleted_upgrade":true,"duplicate_title_and_value_rejected_before_triggers":true,"source_facts_unchanged":true,"uncertain_retry_blocked":true,"compound_statement_journal":true}`)
}
