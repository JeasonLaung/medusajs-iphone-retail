package uniquekey_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/holderimport"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/migrations"
	"github.com/go-sql-driver/mysql"
)

func holderFixture(id, external string) rowimport.SourceRow {
	return rowimport.SourceRow{"id": merchantText(id), "provider_id": merchantText("pp_system_default"), "external_id": merchantText(external),
		"email": merchantText("fixture@example.invalid"), "data": {Raw: json.RawMessage(` { } `)}, "metadata": {SQLNull: true},
		"created_at": merchantText("2026-10-27T00:00:00.123456Z"), "updated_at": merchantText("2026-10-27T00:00:00.123456Z"), "deleted_at": {SQLNull: true}}
}

func TestHolderImportRejectsUnreviewedCalls(t *testing.T) {
	i, err := holderimport.New(nil)
	if err == nil || i != nil {
		t.Fatal("nil ring accepted")
	}
	var empty *holderimport.Importer
	if err := empty.Import(context.Background(), nil, "", nil); err != holderimport.ErrPolicy {
		t.Fatal("nil transaction accepted")
	}
	if _, err := holderimport.Disposition(context.Background(), nil, "fixture"); err != holderimport.ErrPolicy {
		t.Fatal("nil hold transaction accepted")
	}
}

func TestHolderImportMySQL(t *testing.T) {
	if os.Getenv("UNIQUEKEY_TEST") != "1" {
		t.Skip("owned isolated artificial MySQL fixture only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "type_fixture"
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("fixture configuration unavailable")
	}
	defer admin.Close()
	for _, stmt := range []string{"CREATE DATABASE holder_fixture CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin", "CREATE TABLE holder_fixture.payment_account_holders LIKE relational_fixture.payment_account_holders"} {
		if _, err := admin.ExecContext(ctx, stmt); err != nil {
			t.Fatal("isolated holder schema unavailable")
		}
	}
	cfg.DBName = "holder_fixture"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("fixture database unavailable")
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	for _, name := range []string{"0002_unique_key_registry.sql", "0003_unique_key_secret_scopes.sql", "0007_migration_holder_dispositions.sql"} {
		ddl, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatal("holder migration missing")
		}
		for _, stmt := range strings.Split(string(ddl), ";") {
			if strings.TrimSpace(stmt) != "" {
				if _, err := db.ExecContext(ctx, stmt); err != nil {
					t.Fatal("holder migration failed")
				}
			}
		}
	}
	old := bytes.Repeat([]byte{91}, 32)
	ring, _ := protected.New("old", map[string][]byte{"old": old})
	importer, _ := holderimport.New(ring)
	snapshot := strings.Repeat("e", 64)
	run := func(i *holderimport.Importer, r rowimport.SourceRow, hash string, commit bool) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := i.Import(ctx, tx, hash, r); err != nil {
			return err
		}
		var id string
		_ = json.Unmarshal(r["id"].Raw, &id)
		if d, err := holderimport.Disposition(ctx, tx, id); err != nil || d != "hold" {
			return errors.New("fixture mandatory hold missing")
		}
		if commit {
			return tx.Commit()
		}
		return nil
	}
	count := func(table, where string, want int) {
		t.Helper()
		var n int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"` "+where).Scan(&n) != nil || n != want {
			t.Fatal("holder fixture count differs")
		}
	}
	t.Run("row_hold_and_long_identity_atomic_rollback", func(t *testing.T) {
		if err := run(importer, holderFixture("rollback", strings.Repeat("历史🚀", 1024)), snapshot, false); err != nil {
			t.Fatal("holder staged import failed", err)
		}
		for _, table := range []string{"payment_account_holders", "migration_holder_dispositions", "unique_key_owners", "unique_key_buckets", "unique_key_claims"} {
			count(table, "", 0)
		}
	})
	t.Run("commit_exact_replay_rotation_and_null_provenance", func(t *testing.T) {
		rotated, _ := protected.New("new", map[string][]byte{"old": old, "new": bytes.Repeat([]byte{93}, 32)})
		next, _ := holderimport.New(rotated)
		for n, metadata := range []rowimport.Cell{{SQLNull: true}, {Raw: json.RawMessage(` null `)}, {Raw: json.RawMessage(` { } `)}} {
			id := []string{"base", "json-null", "json-object"}[n]
			r := holderFixture(id, "legacy-"+id)
			r["metadata"] = metadata
			if err := run(importer, r, snapshot, true); err != nil {
				t.Fatal("holder commit failed", err)
			}
			var before string
			if db.QueryRowContext(ctx, "SELECT data FROM payment_account_holders WHERE id=?", id).Scan(&before) != nil {
				t.Fatal("holder encrypted readback unavailable")
			}
			if err := run(next, r, snapshot, true); err != nil {
				t.Fatal("holder rotated replay failed", err)
			}
			var after string
			var meta sql.NullString
			if db.QueryRowContext(ctx, "SELECT data,metadata FROM payment_account_holders WHERE id=?", id).Scan(&after, &meta) != nil || before != after {
				t.Fatal("holder replay changed first ciphertext")
			}
			plain, err := rowimport.OpenAccountHolderJSON(ring, id, "pp_system_default", "data", after)
			if err != nil || string(plain) != ` { } ` {
				t.Fatal("holder SQL encrypted JSON bytes changed")
			}
			if metadata.SQLNull {
				if meta.Valid {
					t.Fatal("holder SQL NULL became JSON")
				}
			} else {
				p, err := rowimport.OpenAccountHolderJSON(ring, id, "pp_system_default", "metadata", meta.String)
				if err != nil || !bytes.Equal(p, metadata.Raw) {
					t.Fatal("holder JSON NULL/whitespace changed")
				}
			}
		}
		withoutOld, _ := protected.New("new", map[string][]byte{"new": bytes.Repeat([]byte{93}, 32)})
		missing, _ := holderimport.New(withoutOld)
		if !errors.Is(run(missing, holderFixture("base", "legacy-base"), snapshot, true), holderimport.ErrConflict) {
			t.Fatal("holder missing old key accepted")
		}
		count("payment_account_holders", "", 3)
		count("migration_holder_dispositions", "", 3)
		count("unique_key_claims", "", 3)
	})
	t.Run("eight_way_same_actor_replay", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan error, 8)
		for n := 0; n < 8; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- run(importer, holderFixture("concurrent", "legacy-concurrent"), snapshot, true)
			}()
		}
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatal("holder concurrent replay failed", err)
			}
		}
		count("payment_account_holders", "WHERE id='concurrent'", 1)
		count("migration_holder_dispositions", "WHERE source_id='concurrent'", 1)
		count("unique_key_claims", "WHERE owner='concurrent'", 1)
	})
	t.Run("identity_conflicts_and_exact_no_pad_values", func(t *testing.T) {
		if !errors.Is(run(importer, holderFixture("duplicate", "legacy-base"), snapshot, true), holderimport.ErrConflict) {
			t.Fatal("holder external identity duplicate accepted")
		}
		for n, external := range []string{"legacy-base ", "Legacy-base", strings.Repeat("同前缀", 1024) + "A", strings.Repeat("同前缀", 1024) + "B"} {
			r := holderFixture("distinct-"+[]string{"space", "case", "long-a", "long-b"}[n], external)
			if err := run(importer, r, snapshot, true); err != nil {
				t.Fatal("holder distinct full identity rejected", err)
			}
		}
		deleted := holderFixture("deleted-duplicate", "legacy-base")
		deleted["deleted_at"] = merchantText("2026-10-27T01:00:00Z")
		if err := run(importer, deleted, snapshot, true); err != nil {
			t.Fatal("holder deleted identity should not claim", err)
		}
		count("migration_holder_dispositions", "WHERE source_id='duplicate'", 0)
		count("unique_key_claims", "WHERE owner='deleted-duplicate'", 0)
	})
	t.Run("eight_way_competing_external_identity_has_one_winner", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan error, 8)
		for n := 0; n < 8; n++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				id := "competing-" + string(rune('a'+index))
				results <- run(importer, holderFixture(id, "single-provider-actor"), snapshot, true)
			}(n)
		}
		wg.Wait()
		close(results)
		winners := 0
		for err := range results {
			if err == nil {
				winners++
			} else if !errors.Is(err, holderimport.ErrConflict) {
				t.Fatal("holder competing claim returned unexpected failure", err)
			}
		}
		if winners != 1 {
			t.Fatal("holder external identity has multiple owners")
		}
		count("payment_account_holders", "WHERE id LIKE 'competing-%'", 1)
		count("migration_holder_dispositions", "WHERE source_id LIKE 'competing-%'", 1)
		count("unique_key_claims", "WHERE owner LIKE 'competing-%'", 1)
	})
	t.Run("source_snapshot_ordinary_target_and_valid_reseal_conflicts", func(t *testing.T) {
		r := holderFixture("base", "legacy-base")
		if !errors.Is(run(importer, r, strings.Repeat("f", 64), true), holderimport.ErrConflict) {
			t.Fatal("holder changed snapshot accepted")
		}
		r["email"] = merchantText("changed@example.invalid")
		if !errors.Is(run(importer, r, snapshot, true), holderimport.ErrConflict) {
			t.Fatal("holder changed source accepted")
		}
		if _, err := db.ExecContext(ctx, "UPDATE payment_account_holders SET external_id='changed' WHERE id='base'"); err != nil {
			t.Fatal("holder tamper unavailable")
		}
		if !errors.Is(run(importer, holderFixture("base", "legacy-base"), snapshot, true), holderimport.ErrConflict) {
			t.Fatal("holder changed ordinary target accepted")
		}
		if _, err := db.ExecContext(ctx, "UPDATE payment_account_holders SET external_id='legacy-base' WHERE id='base'"); err != nil {
			t.Fatal("holder restore unavailable")
		}
		var original string
		if db.QueryRowContext(ctx, "SELECT data FROM payment_account_holders WHERE id='base'").Scan(&original) != nil {
			t.Fatal("holder original cipher unavailable")
		}
		sealed, err := ring.Seal(protected.Context{Purpose: "account-holder-history.pp_system_default", Table: "payment_account_holders", Column: "data", Row: "base"}, []byte(` { } `))
		if err != nil {
			t.Fatal("holder fixture reseal unavailable")
		}
		if _, err := db.ExecContext(ctx, "UPDATE payment_account_holders SET data=? WHERE id='base'", "ers1."+base64.StdEncoding.EncodeToString(sealed)); err != nil {
			t.Fatal("holder reseal write unavailable")
		}
		if !errors.Is(run(importer, holderFixture("base", "legacy-base"), snapshot, true), holderimport.ErrConflict) {
			t.Fatal("holder valid reseal bypassed target digest")
		}
		if _, err := db.ExecContext(ctx, "UPDATE payment_account_holders SET data=? WHERE id='base'", original); err != nil {
			t.Fatal("holder cipher restore unavailable")
		}
	})
	t.Run("business_failure_rolls_back_identity_and_hold", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, "ALTER TABLE payment_account_holders ADD CONSTRAINT ck_fixture_holder_reject CHECK(id <> 'business-fail')"); err != nil {
			t.Fatal("holder failure check unavailable")
		}
		if !errors.Is(run(importer, holderFixture("business-fail", "claim-after-failure"), snapshot, true), holderimport.ErrStorage) {
			t.Fatal("holder business failure not propagated")
		}
		count("migration_holder_dispositions", "WHERE source_id='business-fail'", 0)
		count("unique_key_claims", "WHERE owner='business-fail'", 0)
		count("unique_key_owners", "WHERE owner='business-fail'", 0)
		if err := run(importer, holderFixture("after-failure", "claim-after-failure"), snapshot, true); err != nil {
			t.Fatal("holder failed claim survived rollback", err)
		}
	})
	t.Run("missing_target_missing_hold_and_release_denied", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, "DELETE FROM payment_account_holders WHERE id='base'"); err != nil {
			t.Fatal("holder removal fixture unavailable")
		}
		if !errors.Is(run(importer, holderFixture("base", "legacy-base"), snapshot, true), holderimport.ErrConflict) {
			t.Fatal("holder missing target recreated")
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal("holder disposition transaction unavailable")
		}
		defer tx.Rollback()
		if _, err := holderimport.Disposition(ctx, tx, "missing"); err == nil {
			t.Fatal("holder missing hold accepted")
		}
		if _, err := tx.ExecContext(ctx, "UPDATE migration_holder_dispositions SET disposition='allow' WHERE source_id='base'"); err == nil {
			t.Fatal("holder release accepted")
		}
		bad := holderFixture("bad-policy", "bad-policy")
		bad["provider_id"] = merchantText("pp_other_other")
		if err := importer.Import(ctx, tx, snapshot, bad); err != holderimport.ErrPolicy {
			t.Fatal("holder unreviewed provider entered SQL importer")
		}
		if err := importer.Import(ctx, tx, "NOT-A-SNAPSHOT", holderFixture("invalid-sha", "invalid-sha")); err != holderimport.ErrPolicy {
			t.Fatal("holder invalid provenance accepted")
		}
	})
}
