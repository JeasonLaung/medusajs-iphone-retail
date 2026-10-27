package uniquekey_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/merchantimport"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/migrations"
	"github.com/go-sql-driver/mysql"
)

func merchantText(s string) rowimport.Cell { b, _ := json.Marshal(s); return rowimport.Cell{Raw: b} }

// Entirely artificial merchant configuration; no application environment or keys.
func merchantFixture(table, id string) rowimport.SourceRow {
	r := rowimport.SourceRow{"id": merchantText(id), "created_at": merchantText("2026-10-27T00:00:00.123456Z"),
		"updated_at": merchantText("2026-10-27T00:00:00.123456Z"), "deleted_at": {SQLNull: true}}
	fields := map[string]map[string]string{
		"mail_account":      {"name": "fixture", "email": id + "@example.invalid", "reply_to": "", "api_key": "", "webhook_secret": ""},
		"seo_provider":      {"provider": "chatgpt", "api_key": "", "base_url": "", "model": "fixture", "wire_api": "responses"},
		"seo_setting":       {"key": strings.Repeat("配置", 512) + id, "value": " 原文 fixture ", "group": "fixture"},
		"webhook_endpoint":  {"name": "fixture", "provider": "slack", "url": "", "secret": "", "token": "", "keyword": "", "language": "en", "last_status": ""},
		"mollie_config":     {"environment": "test", "api_key": "", "redirect_url": ""},
		"paypal_connection": {"environment": "live", "status": "connected", "seller_client_secret": " fixture secret ", "auth_code": "fixture-code"},
	}
	for k, v := range fields[table] {
		r[k] = merchantText(v)
	}
	switch table {
	case "mail_account", "mollie_config":
		r["enabled"] = rowimport.Cell{Raw: json.RawMessage(`true`)}
	case "seo_setting":
		r["is_secret"] = rowimport.Cell{Raw: json.RawMessage(`false`)}
	case "webhook_endpoint":
		r["enabled"] = rowimport.Cell{Raw: json.RawMessage(`true`)}
		r["events"] = rowimport.Cell{Raw: json.RawMessage(`[]`)}
		r["last_sent_at"] = rowimport.Cell{SQLNull: true}
	case "paypal_connection":
		for _, k := range []string{"shared_id", "seller_client_id", "seller_merchant_id", "seller_email", "app_access_token", "app_access_token_expires_at", "metadata"} {
			r[k] = rowimport.Cell{SQLNull: true}
		}
	case "paypal_settings":
		r["data"] = rowimport.Cell{Raw: json.RawMessage(`{"api_details":{"currency_code":"EUR"}}`)}
	}
	return r
}

func TestMerchantFixtureConversion(t *testing.T) {
	ring, _ := protected.New("fixture", map[string][]byte{"fixture": bytes.Repeat([]byte{83}, 32)})
	converter, _ := rowimport.NewConfigSecrets(ring, nil)
	for _, table := range []string{"mail_account", "seo_provider", "seo_setting", "webhook_endpoint", "mollie_config", "paypal_connection", "paypal_settings"} {
		if _, err := converter.Convert(table, merchantFixture(table, "merchant-"+table)); err != nil {
			t.Fatal("artificial merchant source does not satisfy reviewed plan", table, err)
		}
	}
}

func TestMerchantImportMySQL(t *testing.T) {
	if os.Getenv("UNIQUEKEY_TEST") != "1" {
		t.Skip("isolated artificial MySQL fixture only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "relational_fixture"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("fixture config failed")
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	for _, file := range []string{"0002_unique_key_registry.sql", "0003_unique_key_secret_scopes.sql", "0006_migration_merchant_dispositions.sql"} {
		ddl, err := migrations.Files.ReadFile(file)
		if err != nil {
			t.Fatal("fixture migration missing")
		}
		for _, stmt := range strings.Split(string(ddl), ";") {
			if strings.TrimSpace(stmt) != "" {
				if _, err := db.ExecContext(ctx, stmt); err != nil {
					t.Fatal("fixture migration failed", err)
				}
			}
		}
	}
	old := bytes.Repeat([]byte{83}, 32)
	ring, _ := protected.New("first", map[string][]byte{"first": old})
	importer, _ := merchantimport.New(ring, nil, nil)
	snapshot := strings.Repeat("c", 64)
	run := func(i *merchantimport.Importer, table string, r rowimport.SourceRow, hash string, commit bool) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := i.Import(ctx, tx, hash, table, r); err != nil {
			return err
		}
		d, err := merchantimport.Disposition(ctx, tx, table, stringMustID(r))
		if err != nil || d != "hold" {
			return errors.New("fixture hold missing")
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
			t.Fatal("fixture cardinality mismatch")
		}
	}
	t.Run("all_seven_row_hold_and_identity_rollback", func(t *testing.T) {
		for _, table := range []string{"mail_account", "seo_provider", "seo_setting", "webhook_endpoint", "mollie_config", "paypal_connection", "paypal_settings"} {
			if err := run(importer, table, merchantFixture(table, "merchant-"+table), snapshot, false); err != nil {
				t.Fatal("reviewed merchant rollback failed", table, err)
			}
		}
		count("SELECT COUNT(*) FROM migration_merchant_dispositions", 0)
		count("SELECT COUNT(*) FROM unique_key_owners", 0)
		count("SELECT COUNT(*) FROM unique_key_claims", 0)
		for _, table := range []string{"mail_accounts", "seo_providers", "seo_settings", "webhook_endpoints", "mollie_configs", "paypal_connections", "paypal_settings"} {
			count("SELECT COUNT(*) FROM `"+table+"` WHERE id LIKE 'merchant-%'", 0)
		}
	})
	t.Run("all_seven_commit_replay_rotation_and_binary_protection", func(t *testing.T) {
		rotated, _ := protected.New("second", map[string][]byte{"first": old, "second": bytes.Repeat([]byte{89}, 32)})
		rotatedImporter, _ := merchantimport.New(rotated, nil, nil)
		for _, table := range []string{"mail_account", "seo_provider", "seo_setting", "webhook_endpoint", "mollie_config", "paypal_connection", "paypal_settings"} {
			r := merchantFixture(table, "merchant-"+table)
			if err := run(importer, table, r, snapshot, true); err != nil {
				t.Fatal("reviewed merchant commit failed", table, err)
			}
			if err := run(rotatedImporter, table, r, snapshot, true); err != nil {
				t.Fatal("rotated replay failed", table, err)
			}
		}
		count("SELECT COUNT(*) FROM migration_merchant_dispositions", 7)
		count("SELECT COUNT(*) FROM unique_key_claims", 2)
		var sealed []byte
		if db.QueryRowContext(ctx, "SELECT value FROM seo_settings WHERE id='merchant-seo_setting'").Scan(&sealed) != nil {
			t.Fatal("protected readback failed")
		}
		plain, err := ring.Open(protected.Context{Purpose: "merchant-config", Table: "seo_settings", Column: "value", Row: "merchant-seo_setting"}, sealed)
		if err != nil || string(plain) != " 原文 fixture " {
			t.Fatal("protected plaintext or original key changed")
		}
		withoutOld, _ := protected.New("second", map[string][]byte{"second": bytes.Repeat([]byte{89}, 32)})
		bad, _ := merchantimport.New(withoutOld, nil, nil)
		if !errors.Is(run(bad, "seo_setting", merchantFixture("seo_setting", "merchant-seo_setting"), snapshot, true), merchantimport.ErrConflict) {
			t.Fatal("missing old key accepted")
		}
	})
	t.Run("eight_way_exact_replay", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan error, 8)
		for n := 0; n < 8; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- run(importer, "seo_setting", merchantFixture("seo_setting", "merchant-concurrent"), snapshot, true)
			}()
		}
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatal("concurrent replay failed", err)
			}
		}
		count("SELECT COUNT(*) FROM seo_settings WHERE id='merchant-concurrent'", 1)
		count("SELECT COUNT(*) FROM migration_merchant_dispositions WHERE source_id='merchant-concurrent'", 1)
	})
	t.Run("identity_source_snapshot_and_target_conflicts", func(t *testing.T) {
		for _, table := range []string{"mail_account", "seo_setting"} {
			r := merchantFixture(table, "merchant-other-"+table)
			original := merchantFixture(table, "merchant-"+table)
			field := "key"
			if table == "mail_account" {
				field = "email"
			}
			r[field] = original[field]
			if !errors.Is(run(importer, table, r, snapshot, true), merchantimport.ErrConflict) {
				t.Fatal("duplicate identity accepted")
			}
		}
		r := merchantFixture("seo_setting", "merchant-seo_setting")
		if !errors.Is(run(importer, "seo_setting", r, strings.Repeat("d", 64), true), merchantimport.ErrConflict) {
			t.Fatal("changed snapshot accepted")
		}
		r["value"] = merchantText("changed")
		if !errors.Is(run(importer, "seo_setting", r, snapshot, true), merchantimport.ErrConflict) {
			t.Fatal("changed source accepted")
		}
		if _, err := db.ExecContext(ctx, "UPDATE seo_settings SET `group`='tampered' WHERE id='merchant-seo_setting'"); err != nil {
			t.Fatal("fixture tamper failed")
		}
		if !errors.Is(run(importer, "seo_setting", merchantFixture("seo_setting", "merchant-seo_setting"), snapshot, true), merchantimport.ErrConflict) {
			t.Fatal("changed target accepted")
		}
		if _, err := db.ExecContext(ctx, "UPDATE seo_settings SET `group`='fixture' WHERE id='merchant-seo_setting'"); err != nil {
			t.Fatal("fixture restore failed")
		}
		count("SELECT COUNT(*) FROM migration_merchant_dispositions WHERE source_id LIKE 'merchant-other-%'", 0)
	})
	t.Run("valid_reseal_tampered_digest_and_missing_target_conflict", func(t *testing.T) {
		var original []byte
		if db.QueryRowContext(ctx, "SELECT value FROM seo_settings WHERE id='merchant-seo_setting'").Scan(&original) != nil {
			t.Fatal("fixture ciphertext unavailable")
		}
		resealed, err := ring.Seal(protected.Context{Purpose: "merchant-config", Table: "seo_settings", Column: "value", Row: "merchant-seo_setting"}, []byte(" 原文 fixture "))
		if err != nil {
			t.Fatal("fixture reseal failed")
		}
		if _, err := db.ExecContext(ctx, "UPDATE seo_settings SET value=? WHERE id='merchant-seo_setting'", resealed); err != nil {
			t.Fatal("fixture cipher replacement failed")
		}
		if !errors.Is(run(importer, "seo_setting", merchantFixture("seo_setting", "merchant-seo_setting"), snapshot, true), merchantimport.ErrConflict) {
			t.Fatal("valid reseal bypassed recorded target digest")
		}
		if _, err := db.ExecContext(ctx, "UPDATE seo_settings SET value=? WHERE id='merchant-seo_setting'", original); err != nil {
			t.Fatal("fixture cipher restore failed")
		}
		var digest []byte
		if db.QueryRowContext(ctx, "SELECT target_row_sha256 FROM migration_merchant_dispositions WHERE source_id='merchant-seo_setting'").Scan(&digest) != nil {
			t.Fatal("fixture digest unavailable")
		}
		if _, err := db.ExecContext(ctx, "UPDATE migration_merchant_dispositions SET target_row_sha256=? WHERE source_id='merchant-seo_setting'", make([]byte, 32)); err != nil {
			t.Fatal("fixture digest replacement failed")
		}
		if !errors.Is(run(importer, "seo_setting", merchantFixture("seo_setting", "merchant-seo_setting"), snapshot, true), merchantimport.ErrConflict) {
			t.Fatal("changed target digest accepted")
		}
		if _, err := db.ExecContext(ctx, "UPDATE migration_merchant_dispositions SET target_row_sha256=? WHERE source_id='merchant-seo_setting'", digest); err != nil {
			t.Fatal("fixture digest restore failed")
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM paypal_settings WHERE id='merchant-paypal_settings'"); err != nil {
			t.Fatal("fixture target removal failed")
		}
		if !errors.Is(run(importer, "paypal_settings", merchantFixture("paypal_settings", "merchant-paypal_settings"), snapshot, true), merchantimport.ErrConflict) {
			t.Fatal("missing target silently recreated")
		}
	})
	t.Run("failed_business_insert_rolls_back_hold_and_identity", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, "ALTER TABLE seo_settings ADD CONSTRAINT ck_fixture_merchant_failure CHECK(id <> 'merchant-fail')"); err != nil {
			t.Fatal("fixture check failed")
		}
		r := merchantFixture("seo_setting", "merchant-fail")
		if !errors.Is(run(importer, "seo_setting", r, snapshot, true), merchantimport.ErrStorage) {
			t.Fatal("business insert failure not propagated")
		}
		count("SELECT COUNT(*) FROM migration_merchant_dispositions WHERE source_id='merchant-fail'", 0)
		count("SELECT COUNT(*) FROM unique_key_owners WHERE owner='merchant-fail'", 0)
		count("SELECT COUNT(*) FROM unique_key_claims WHERE owner='merchant-fail'", 0)
		r["id"] = merchantText("merchant-after-failure")
		if err := run(importer, "seo_setting", r, snapshot, true); err != nil {
			t.Fatal("rolled-back identity remained locked", err)
		}
	})
	t.Run("missing_unknown_and_release_fail_closed", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal("fixture transaction unavailable")
		}
		defer tx.Rollback()
		if _, err := merchantimport.Disposition(ctx, tx, "seo_setting", "missing"); err == nil {
			t.Fatal("missing hold accepted")
		}
		if _, err := merchantimport.Disposition(ctx, tx, "mail_contact", "fixture"); err == nil {
			t.Fatal("unreviewed capability accepted")
		}
		if err := importer.Import(ctx, tx, snapshot, "mail_contact", nil); err != merchantimport.ErrPolicy {
			t.Fatal("contact entered merchant importer")
		}
		if _, err := tx.ExecContext(ctx, "UPDATE migration_merchant_dispositions SET disposition='allow' WHERE source_id='merchant-seo_setting'"); err == nil {
			t.Fatal("hold release accepted")
		}
	})
}

func stringMustID(r rowimport.SourceRow) string {
	var id string
	_ = json.Unmarshal(r["id"].Raw, &id)
	return id
}
