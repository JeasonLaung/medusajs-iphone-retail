package uniquekey_test

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

	"github.com/eurorefurb/retail/internal/catalogimport"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/migrations"
	"github.com/go-sql-driver/mysql"
)

func catalogTag(id, value string) rowimport.SourceRow {
	return rowimport.SourceRow{
		"id": merchantText(id), "value": merchantText(value), "metadata": {Raw: json.RawMessage(` {"n":9007199254740993,"n":1.2300} `)},
		"created_at": merchantText("2026-10-27T00:00:00.123456Z"), "updated_at": merchantText("2026-10-27T00:00:00.123456Z"), "deleted_at": {SQLNull: true}, "external_id": {SQLNull: true},
	}
}
func catalogPrice() rowimport.SourceRow {
	return rowimport.SourceRow{
		"id": merchantText("price-exact"), "title": {SQLNull: true}, "price_set_id": merchantText("price-parent"), "currency_code": merchantText("eur"),
		"amount": {Raw: json.RawMessage(`"1.123456789012345678"`)}, "raw_amount": {Raw: json.RawMessage(`{"value":"1.123456789012345678","precision":20}`)},
		"rules_count": {Raw: json.RawMessage(`0`)}, "created_at": merchantText("2026-10-27T00:00:00Z"), "updated_at": merchantText("2026-10-27T00:00:00Z"),
		"deleted_at": {SQLNull: true}, "price_list_id": {SQLNull: true}, "min_quantity": {SQLNull: true}, "max_quantity": {SQLNull: true}, "raw_min_quantity": {SQLNull: true}, "raw_max_quantity": {SQLNull: true},
	}
}

func TestCatalogImportRejectsUnreviewedCalls(t *testing.T) {
	if err := catalogimport.Import(t.Context(), nil, "", "product", nil); err != catalogimport.ErrPolicy {
		t.Fatal("nil transaction accepted")
	}
	if _, err := rowimport.Convert("product_tag", catalogTag("id", "value")); err != nil {
		t.Fatal("catalog tag fixture invalid")
	}
	if _, err := rowimport.Convert("price", catalogPrice()); err != nil {
		t.Fatal("catalog price fixture invalid")
	}
}

func TestCatalogImportMySQL(t *testing.T) {
	if os.Getenv("UNIQUEKEY_TEST") != "1" {
		t.Skip("owned isolated artificial MySQL only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "type_fixture"
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("catalog fixture invalid")
	}
	defer admin.Close()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE catalog_import_fixture CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin"); err != nil {
		t.Fatal("catalog fixture unavailable")
	}
	for _, table := range []string{"catalog_tags", "catalog_category_products", "pricing_prices"} {
		if _, err := admin.ExecContext(ctx, "CREATE TABLE catalog_import_fixture.`"+table+"` LIKE relational_fixture.`"+table+"`"); err != nil {
			t.Fatal("actual reviewed catalog table unavailable")
		}
	}
	cfg.DBName = "catalog_import_fixture"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("catalog fixture connection unavailable")
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	for _, name := range []string{"0002_unique_key_registry.sql", "0003_unique_key_secret_scopes.sql", "0008_migration_catalog_rows.sql"} {
		ddl, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatal("catalog migration missing")
		}
		for _, stmt := range strings.Split(string(ddl), ";") {
			if strings.TrimSpace(stmt) != "" {
				if _, err := db.ExecContext(ctx, stmt); err != nil {
					t.Fatal("catalog migration failed")
				}
			}
		}
	}
	snapshot := strings.Repeat("a", 64)
	run := func(table string, row rowimport.SourceRow, hash string, commit bool) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := catalogimport.Import(ctx, tx, hash, table, row); err != nil {
			return err
		}
		if commit {
			return tx.Commit()
		}
		return nil
	}
	count := func(query string, want int) {
		t.Helper()
		var n int
		if db.QueryRowContext(ctx, query).Scan(&n) != nil || n != want {
			t.Fatal("catalog fixture cardinality differs")
		}
	}
	t.Run("row_ledger_full_unique_claim_rollback", func(t *testing.T) {
		if err := run("product_tag", catalogTag("rollback", strings.Repeat("长值🚀", 1024)), snapshot, false); err != nil {
			t.Fatal("catalog rollback staging failed", err)
		}
		for _, table := range []string{"catalog_tags", "migration_catalog_rows", "unique_key_owners", "unique_key_buckets", "unique_key_claims"} {
			count("SELECT COUNT(*) FROM "+table, 0)
		}
	})
	t.Run("complete_row_json_null_and_exact_replay", func(t *testing.T) {
		row := catalogTag("base", "catalog-long-value")
		if err := run("product_tag", row, snapshot, true); err != nil {
			t.Fatal("catalog commit failed", err)
		}
		if err := run("product_tag", row, snapshot, true); err != nil {
			t.Fatal("catalog exact replay failed", err)
		}
		var metadata string
		var external sql.NullString
		if db.QueryRowContext(ctx, "SELECT metadata,external_id FROM catalog_tags WHERE id='base'").Scan(&metadata, &external) != nil || metadata != string(row["metadata"].Raw) || external.Valid {
			t.Fatal("catalog exact JSON/SQL NULL changed")
		}
		count("SELECT COUNT(*) FROM migration_catalog_rows", 1)
		count("SELECT COUNT(*) FROM unique_key_claims", 1)
	})
	t.Run("composite_primary_key_and_renamed_columns", func(t *testing.T) {
		for _, pair := range [][2]string{{"a", "bc"}, {"ab", "c"}} {
			row := rowimport.SourceRow{"product_id": merchantText(pair[0]), "product_category_id": merchantText(pair[1])}
			if err := run("product_category_product", row, snapshot, true); err != nil {
				t.Fatal("composite catalog insert failed", err)
			}
			if err := run("product_category_product", row, snapshot, true); err != nil {
				t.Fatal("composite catalog replay failed", err)
			}
		}
		count("SELECT COUNT(*) FROM catalog_category_products", 2)
		count("SELECT COUNT(*) FROM migration_catalog_rows WHERE source_table='product_category_product'", 2)
	})
	t.Run("decimal_comparison_cannot_hide_last_place_change", func(t *testing.T) {
		row := catalogPrice()
		if err := run("price", row, snapshot, true); err != nil {
			t.Fatal("exact price insert failed", err)
		}
		if err := run("price", row, snapshot, true); err != nil {
			t.Fatal("exact price replay failed", err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE pricing_prices SET amount=1.123456789012345679 WHERE id='price-exact'"); err != nil {
			t.Fatal("decimal tamper failed")
		}
		if !errors.Is(run("price", row, snapshot, true), catalogimport.ErrConflict) {
			t.Fatal("decimal comparison rounded away last-place drift")
		}
	})
	t.Run("snapshot_source_target_ledger_and_missing_row_conflicts", func(t *testing.T) {
		row := catalogTag("base", "catalog-long-value")
		if !errors.Is(run("product_tag", row, strings.Repeat("b", 64), true), catalogimport.ErrConflict) {
			t.Fatal("changed catalog snapshot accepted")
		}
		changed := catalogTag("base", "changed")
		if !errors.Is(run("product_tag", changed, snapshot, true), catalogimport.ErrConflict) {
			t.Fatal("changed source accepted")
		}
		if _, err := db.ExecContext(ctx, "UPDATE catalog_tags SET metadata='null' WHERE id='base'"); err != nil {
			t.Fatal("catalog target change unavailable")
		}
		if !errors.Is(run("product_tag", row, snapshot, true), catalogimport.ErrConflict) {
			t.Fatal("changed target accepted")
		}
		if _, err := db.ExecContext(ctx, "UPDATE catalog_tags SET metadata=? WHERE id='base'", string(row["metadata"].Raw)); err != nil {
			t.Fatal("catalog target restore unavailable")
		}
		if !errors.Is(run("product_tag", catalogTag("duplicate", "catalog-long-value"), snapshot, true), catalogimport.ErrConflict) {
			t.Fatal("duplicate full tag identity accepted")
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM catalog_tags WHERE id='base'"); err != nil {
			t.Fatal("catalog target removal unavailable")
		}
		if !errors.Is(run("product_tag", row, snapshot, true), catalogimport.ErrConflict) {
			t.Fatal("missing catalog row recreated")
		}
	})
	t.Run("eight_way_same_row_replay", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan error, 8)
		for n := 0; n < 8; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- run("product_tag", catalogTag("concurrent", "concurrent-value"), snapshot, true)
			}()
		}
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatal("catalog concurrent replay failed", err)
			}
		}
		count("SELECT COUNT(*) FROM catalog_tags WHERE id='concurrent'", 1)
		count("SELECT COUNT(*) FROM unique_key_claims WHERE owner='concurrent'", 1)
	})
	t.Run("business_sql_failure_and_unknown_domain_fail_closed", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, "ALTER TABLE catalog_tags ADD CONSTRAINT ck_fixture_catalog_failure CHECK(id <> 'fail')"); err != nil {
			t.Fatal("catalog business failure fixture unavailable")
		}
		if !errors.Is(run("product_tag", catalogTag("fail", "failed-value"), snapshot, true), catalogimport.ErrStorage) {
			t.Fatal("catalog SQL failure swallowed")
		}
		count("SELECT COUNT(*) FROM unique_key_owners WHERE owner='fail'", 0)
		if err := run("product_tag", catalogTag("after-fail", "failed-value"), snapshot, true); err != nil {
			t.Fatal("catalog failed identity survived rollback", err)
		}
		if err := run("account_holder", nil, snapshot, true); err != catalogimport.ErrPolicy {
			t.Fatal("catalog imported protected domain")
		}
		if err := run("product_tag", catalogTag("invalid-hash", "invalid-hash"), "not-a-hash", true); err != catalogimport.ErrPolicy {
			t.Fatal("catalog invalid provenance accepted")
		}
	})
}
