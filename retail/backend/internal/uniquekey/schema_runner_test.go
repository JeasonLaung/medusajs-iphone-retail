package uniquekey_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/go-sql-driver/mysql"
)

func TestSchemaRunnerMySQL(t *testing.T) {
	if os.Getenv("UNIQUEKEY_TEST") != "1" {
		t.Skip("owned artificial disconnected MySQL only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "type_fixture"
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("schema fixture connection invalid")
	}
	defer admin.Close()
	open := func(name string) *sql.DB {
		t.Helper()
		if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin"); err != nil {
			t.Fatal("schema fixture database unavailable")
		}
		c := *cfg
		c.DBName = name
		db, err := sql.Open("mysql", c.FormatDSN())
		if err != nil {
			t.Fatal("schema fixture database invalid")
		}
		t.Cleanup(func() { db.Close() })
		return db
	}
	count := func(db *sql.DB, query string, want int) {
		t.Helper()
		var n int
		if db.QueryRowContext(ctx, query).Scan(&n) != nil || n != want {
			t.Fatal("schema fixture count differs")
		}
	}
	t.Run("all_embedded_migrations_repeat_and_checksum_rejection", func(t *testing.T) {
		db := open("schema_fixture")
		p, err := schema.Load(migrations.Files)
		if err != nil {
			t.Fatal("embedded plan invalid")
		}
		r, err := schema.Apply(ctx, db, p)
		if err != nil || r.Applied != 7 || r.Skipped != 0 {
			t.Fatal("embedded explicit migration execution failed", err)
		}
		r, err = schema.Apply(ctx, db, p)
		if err != nil || r.Applied != 0 || r.Skipped != 7 {
			t.Fatal("embedded migration repeat failed", err)
		}
		count(db, "SELECT COUNT(*) FROM schema_migrations", 7)
		count(db, "SELECT COUNT(*) FROM schema_migration_steps WHERE status='applied' AND inflight=FALSE AND next_statement=statement_count", 7)
		entries, _ := migrations.Files.ReadDir(".")
		changed := fstest.MapFS{}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			b, err := migrations.Files.ReadFile(e.Name())
			if err != nil {
				t.Fatal("fixture migration read failed")
			}
			changed[e.Name()] = &fstest.MapFile{Data: b}
		}
		for name, f := range changed {
			if len(name) > 5 && name[:5] == "0001_" {
				f.Data = append(append([]byte{}, f.Data...), []byte("\n-- checksum-only change\n")...)
			}
		}
		bad, err := schema.Load(changed)
		if err != nil {
			t.Fatal("changed valid fixture plan invalid")
		}
		if _, err := schema.Apply(ctx, db, bad); !errors.Is(err, schema.ErrDrift) {
			t.Fatal("changed applied checksum accepted")
		}
		changed["0000_earlier.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE must_not_exist(id INT);")}
		// Restore the original checksum before isolating the order check.
		for name := range changed {
			if len(name) > 5 && name[:5] == "0001_" {
				b, _ := migrations.Files.ReadFile(name)
				changed[name] = &fstest.MapFile{Data: b}
			}
		}
		order, err := schema.Load(changed)
		if err != nil {
			t.Fatal("earlier fixture plan invalid")
		}
		if _, err := schema.Apply(ctx, db, order); !errors.Is(err, schema.ErrDrift) {
			t.Fatal("earlier unapplied migration accepted")
		}
		count(db, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='schema_fixture' AND TABLE_NAME='must_not_exist'", 0)
	})
	t.Run("implicit_ddl_failure_blocks_replay_and_explicit_reconciliation", func(t *testing.T) {
		db := open("schema_failure_fixture")
		p, err := schema.Load(fstest.MapFS{"0001_failure.sql": {Data: []byte("-- ; in comment\nCREATE TABLE acknowledged(id INT PRIMARY KEY); INSERT INTO absent VALUES(1);")}})
		if err != nil {
			t.Fatal("failure fixture plan invalid")
		}
		if _, err := schema.Apply(ctx, db, p); !errors.Is(err, schema.ErrUncertain) {
			t.Fatal("DDL failure not held for reconciliation", err)
		}
		count(db, "SELECT COUNT(*) FROM schema_migrations", 0)
		count(db, "SELECT COUNT(*) FROM schema_migration_steps WHERE next_statement=1 AND inflight=TRUE AND status='failed'", 1)
		if _, err := schema.Apply(ctx, db, p); !errors.Is(err, schema.ErrUncertain) {
			t.Fatal("ambiguous DDL blindly replayed")
		}
		// Artificial operator reconciliation only: prove the failed INSERT's
		// target is absent, restore its reviewed dependency, and explicitly
		// acknowledge that this statement had no effect. No production API does this.
		count(db, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='schema_failure_fixture' AND TABLE_NAME='absent'", 0)
		if _, err := db.ExecContext(ctx, "CREATE TABLE absent(id INT PRIMARY KEY)"); err != nil {
			t.Fatal("fixture dependency repair failed")
		}
		if _, err := db.ExecContext(ctx, "UPDATE schema_migration_steps SET inflight=FALSE,status='running' WHERE version='0001_failure.sql' AND next_statement=1"); err != nil {
			t.Fatal("fixture reconciliation failed")
		}
		r, err := schema.Apply(ctx, db, p)
		if err != nil || r.Applied != 1 {
			t.Fatal("acknowledged prefix did not resume", err)
		}
		count(db, "SELECT COUNT(*) FROM absent WHERE id=1", 1)
		r, err = schema.Apply(ctx, db, p)
		if err != nil || r.Skipped != 1 {
			t.Fatal("recovered migration repeat failed")
		}
	})
	t.Run("cross_connection_named_lock_and_release_after_cancel", func(t *testing.T) {
		db := open("schema_lock_fixture")
		p, err := schema.Load(fstest.MapFS{"0001_lock.sql": {Data: []byte("CREATE TABLE lock_result(id INT);")}})
		if err != nil {
			t.Fatal("lock fixture invalid")
		}
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal("lock owner connection invalid")
		}
		defer conn.Close()
		var acquired int
		if conn.QueryRowContext(ctx, "SELECT GET_LOCK('eurorefurb_schema',0)").Scan(&acquired) != nil || acquired != 1 {
			t.Fatal("fixture named lock acquisition failed")
		}
		if _, err := schema.Apply(ctx, db, p); !errors.Is(err, schema.ErrLocked) {
			t.Fatal("second connection bypassed migration lock")
		}
		var released int
		if conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK('eurorefurb_schema')").Scan(&released) != nil || released != 1 {
			t.Fatal("fixture named lock release failed")
		}
		cancelled, stop := context.WithCancel(ctx)
		stop()
		if _, err := schema.Apply(cancelled, db, p); err == nil {
			t.Fatal("cancelled migration accepted")
		}
		if _, err := schema.Apply(ctx, db, p); err != nil {
			t.Fatal("migration lock stranded after cancellation", err)
		}
		if conn.QueryRowContext(ctx, "SELECT GET_LOCK('eurorefurb_schema',0)").Scan(&acquired) != nil || acquired != 1 {
			t.Fatal("successful runner retained pooled named lock")
		}
		if conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK('eurorefurb_schema')").Scan(&released) != nil || released != 1 {
			t.Fatal("fixture final lock release failed")
		}
	})
}
