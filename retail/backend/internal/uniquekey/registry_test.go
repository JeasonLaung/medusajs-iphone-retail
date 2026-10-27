package uniquekey

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/migrations"
	"github.com/go-sql-driver/mysql"
)

func ptr(s string) *string { return &s }

func TestTupleEncoding(t *testing.T) {
	for _, pair := range [][2][]*string{
		{{ptr("ab"), ptr("c")}, {ptr("a"), ptr("bc")}},
		{{ptr("A")}, {ptr("a")}}, {{ptr("a")}, {ptr("a ")}},
		{{ptr("é")}, {ptr("e\u0301")}}, {{ptr("")}, {ptr("\x00")}},
	} {
		a, _ := encode(pair[0])
		b, _ := encode(pair[1])
		if bytes.Equal(a, b) {
			t.Fatal("distinct full tuples collapsed")
		}
	}
	if b, err := encode([]*string{nil}); b != nil || err != nil {
		t.Fatal("NULL must not claim")
	}
	if _, err := encode([]*string{ptr(string([]byte{0xff}))}); err != ErrInvalid {
		t.Fatal("invalid UTF8 accepted")
	}
}

func TestRegistryMySQL(t *testing.T) {
	if os.Getenv("UNIQUEKEY_TEST") != "1" {
		t.Skip("isolated MySQL fixture only")
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "type_fixture"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("fixture connection config failed")
	}
	defer db.Close()
	db.SetMaxOpenConns(12)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ddl, err := migrations.Files.ReadFile("0002_unique_key_registry.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range strings.Split(string(ddl), ";") {
		if strings.TrimSpace(stmt) != "" {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				t.Fatal("fixture DDL failed", err)
			}
		}
	}
	change := func(scope, owner, value string) Change { return Change{scope, owner, true, []*string{ptr(value)}} }
	run := func(changes []Change, collision bool, commit bool) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if collision {
			err = apply(ctx, tx, changes, func([]byte) [32]byte { return [32]byte{} })
		} else {
			err = Apply(ctx, tx, changes)
		}
		if err != nil {
			return err
		}
		if commit {
			return tx.Commit()
		}
		return nil
	}
	ok := func(cs ...Change) {
		t.Helper()
		if err := run(cs, false, true); err != nil {
			t.Fatal(err)
		}
	}
	conflict := func(cs ...Change) {
		t.Helper()
		if err := run(cs, false, true); !errors.Is(err, ErrConflict) {
			t.Fatal("expected exact conflict", err)
		}
	}

	t.Run("full_value_and_collision", func(t *testing.T) {
		long := strings.Repeat("🚀", 1024)
		for _, c := range []Change{change("collision", "one", long+"A"), change("collision", "two", long+"B")} {
			if err := run([]Change{c}, true, true); err != nil {
				t.Fatal(err)
			}
		}
		if err := run([]Change{change("collision", "three", long+"A")}, true, true); err != ErrConflict {
			t.Fatal("collision check lost complete value", err)
		}
	})
	t.Run("case_trailing_space_and_scopes", func(t *testing.T) {
		ok(change("text", "a", "Name"), change("text", "b", "name"), change("text", "c", "name "), change("other", "a", "name"))
		conflict(change("text", "d", "name"))
	})
	t.Run("null_excluded_release_restore", func(t *testing.T) {
		ok(change("life", "a", "same"))
		x := change("life", "a", "same")
		x.Active = false
		ok(x)
		ok(change("life", "b", "same"))
		conflict(change("life", "a", "same"))
		x = change("life", "b", "same")
		x.Components = []*string{nil}
		ok(x)
		ok(change("life", "a", "same"))
		ok(Change{"life", "null", true, []*string{nil}})
	})
	t.Run("atomic_swap_and_failure_rollback", func(t *testing.T) {
		ok(change("swap", "a", "A"), change("swap", "b", "B"))
		ok(change("swap", "a", "B"), change("swap", "b", "A"))
		conflict(change("swap", "a", "A"))
		conflict(change("swap", "c", "B")) // failed replacement preserved a's B
		if err := run([]Change{change("rollback", "a", "value")}, false, false); err != nil {
			t.Fatal(err)
		}
		ok(change("rollback", "b", "value"))
	})
	t.Run("same_owner_retry_and_duplicate_batch", func(t *testing.T) {
		ok(change("retry", "a", "value"))
		ok(change("retry", "a", "value"))
		if err := run([]Change{change("retry", "a", "x"), change("retry", "a", "y")}, false, true); err != ErrInvalid {
			t.Fatal("duplicate owner batch accepted")
		}
	})
	t.Run("repeatable_read_old_snapshot_cannot_hide_new_claim", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM unique_key_claims WHERE scope='snapshot'").Scan(&count); err != nil {
			t.Fatal(err)
		}
		ok(change("snapshot", "new", "claimed-after-snapshot"))
		if err := Apply(ctx, tx, []Change{change("snapshot", "old", "claimed-after-snapshot")}); err != ErrConflict {
			t.Fatal("stale snapshot bypassed current claim", err)
		}
	})
	t.Run("duplicate_full_values_in_one_batch_roll_back", func(t *testing.T) {
		conflict(change("batch", "a", "same"), change("batch", "b", "same"))
		ok(change("batch", "c", "same"))
	})
	t.Run("concurrent_only_one_winner", func(t *testing.T) {
		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				results <- run([]Change{change("race", string(rune('a'+i)), "last-item")}, false, true)
			}(i)
		}
		close(start)
		wg.Wait()
		close(results)
		winners := 0
		for err := range results {
			if err == nil {
				winners++
			} else if !errors.Is(err, ErrConflict) {
				t.Fatal("unexpected concurrent failure", err)
			}
		}
		if winners != 1 {
			t.Fatal("concurrent uniqueness violated", winners)
		}
	})
	t.Run("source_option_adapter_and_domain_row_share_transaction", func(t *testing.T) {
		write := func(id, title string, commit bool) error {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer tx.Rollback()
			err = ApplySourceRow(ctx, tx, "catalog_options", row(map[string]any{"id": id, "title": title, "deleted_at": nil, "is_exclusive": false}))
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, "INSERT INTO relational_fixture.catalog_options(id,title,created_at,updated_at,is_exclusive) VALUES(?,?, '2026-10-27','2026-10-27',false)", id, title)
			if err != nil {
				return err
			}
			if commit {
				return tx.Commit()
			}
			return nil
		}
		if err := write("option-a", "Long exact title ", true); err != nil {
			t.Fatal(err)
		}
		if err := write("option-b", "Long exact title ", true); err != ErrConflict {
			t.Fatal("source adapter duplicate escaped", err)
		}
		if err := write("option-c", "Long exact title", true); err != nil {
			t.Fatal("trailing space collapsed", err)
		}
		if err := write("option-d", "Rollback title", false); err != nil {
			t.Fatal(err)
		}
		if err := write("option-e", "Rollback title", true); err != nil {
			t.Fatal("claim not rolled back with business row", err)
		}
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM relational_fixture.catalog_options WHERE id IN ('option-a','option-b','option-c','option-d','option-e')").Scan(&count); err != nil || count != 3 {
			t.Fatal("business rows mismatch", count, err)
		}
	})
}
