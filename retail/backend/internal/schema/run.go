package schema

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"
)

var (
	ErrStorage   = errors.New("schema migration storage failed")
	ErrLocked    = errors.New("schema migration lock unavailable")
	ErrDrift     = errors.New("schema migration journal or checksum differs")
	ErrUncertain = errors.New("schema migration statement requires explicit reconciliation")
)

type Result struct{ Applied, Skipped int }

// Apply uses one dedicated connection and a named lock, journaling BEFORE each
// implicitly committing MySQL statement. Failure/crash between DDL and its
// recorded completion is deliberately uncertain: retry never guesses or reruns
// it. Such a target needs explicit schema inspection and reconciliation first.
// An acknowledged prefix resumes at the next statement; completed files with
// unchanged checksums are skipped. Error text contains no SQL or driver data.
func Apply(ctx context.Context, db *sql.DB, p *Plan) (result Result, err error) {
	if db == nil || p == nil || len(p.files) == 0 {
		return result, ErrPlan
	}
	conn, e := db.Conn(ctx)
	if e != nil {
		return result, ErrStorage
	}
	defer conn.Close()
	var locked sql.NullInt64
	if e := conn.QueryRowContext(ctx, "SELECT GET_LOCK('eurorefurb_schema', 1)").Scan(&locked); e != nil || !locked.Valid || locked.Int64 != 1 {
		return result, ErrLocked
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var released sql.NullInt64
		if e := conn.QueryRowContext(cleanup, "SELECT RELEASE_LOCK('eurorefurb_schema')").Scan(&released); e != nil || !released.Valid || released.Int64 != 1 {
			// A pooled connection must never retain a session-owned named lock.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			if err == nil {
				err = ErrStorage
			}
		}
	}()
	var autocommit int
	if conn.QueryRowContext(ctx, "SELECT @@autocommit").Scan(&autocommit) != nil || autocommit != 1 {
		return result, ErrStorage
	}
	if _, e := conn.ExecContext(ctx, "SET time_zone = '+00:00'"); e != nil {
		return result, ErrStorage
	}
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (version VARCHAR(191) PRIMARY KEY, checksum CHAR(64) NOT NULL, applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS schema_migration_steps (version VARCHAR(191) PRIMARY KEY, checksum CHAR(64) NOT NULL,
		 statement_count INT NOT NULL, next_statement INT NOT NULL, inflight BOOLEAN NOT NULL, status VARCHAR(16) NOT NULL,
		 updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		 CHECK(statement_count>0 AND next_statement>=0 AND next_statement<=statement_count),
		 CHECK(status IN ('running','failed','applied'))) ENGINE=InnoDB`,
	} {
		if _, e := conn.ExecContext(ctx, ddl); e != nil {
			return result, ErrStorage
		}
	}
	known := map[string]migration{}
	for _, f := range p.files {
		known[f.name] = f
	}
	applied := map[string]bool{}
	rows, e := conn.QueryContext(ctx, "SELECT version,checksum FROM schema_migrations")
	if e != nil {
		return result, ErrStorage
	}
	for rows.Next() {
		var name, checksum string
		if rows.Scan(&name, &checksum) != nil {
			rows.Close()
			return result, ErrStorage
		}
		f, ok := known[name]
		if !ok || f.checksum != checksum {
			rows.Close()
			return result, ErrDrift
		}
		applied[name] = true
	}
	if rows.Err() != nil || rows.Close() != nil {
		return result, ErrStorage
	}
	// Do not silently insert an earlier migration behind an applied later one.
	gap := false
	for _, f := range p.files {
		if !applied[f.name] {
			gap = true
		} else if gap {
			return result, ErrDrift
		}
	}
	type step struct {
		checksum, status string
		count, next      int
		inflight         bool
	}
	steps := map[string]step{}
	rows, e = conn.QueryContext(ctx, "SELECT version,checksum,statement_count,next_statement,inflight,status FROM schema_migration_steps")
	if e != nil {
		return result, ErrStorage
	}
	for rows.Next() {
		var name string
		var s step
		if rows.Scan(&name, &s.checksum, &s.count, &s.next, &s.inflight, &s.status) != nil {
			rows.Close()
			return result, ErrStorage
		}
		f, ok := known[name]
		if !ok || s.checksum != f.checksum || s.count != len(f.statements) || s.next < 0 || s.next > s.count {
			rows.Close()
			return result, ErrDrift
		}
		if applied[name] {
			if s.status != "applied" || s.inflight || s.next != s.count {
				rows.Close()
				return result, ErrDrift
			}
		} else if s.status != "running" && s.status != "failed" {
			rows.Close()
			return result, ErrDrift
		}
		steps[name] = s
	}
	if rows.Err() != nil || rows.Close() != nil {
		return result, ErrStorage
	}
	// Validate EVERY pending step before any business migration executes.
	for name, s := range steps {
		if !applied[name] && (s.inflight || s.status == "failed") {
			return result, ErrUncertain
		}
	}
	firstPending := ""
	for _, f := range p.files {
		if !applied[f.name] {
			firstPending = f.name
			break
		}
	}
	for name := range steps {
		if !applied[name] && name != firstPending {
			return result, ErrDrift
		}
	}
	for _, f := range p.files {
		if applied[f.name] {
			result.Skipped++
			continue
		}
		s, exists := steps[f.name]
		if !exists {
			if _, e := conn.ExecContext(ctx, "INSERT INTO schema_migration_steps(version,checksum,statement_count,next_statement,inflight,status) VALUES(?,?,?,0,FALSE,'running')", f.name, f.checksum, len(f.statements)); e != nil {
				return result, ErrStorage
			}
			s = step{next: 0}
		}
		for j := s.next; j < len(f.statements); j++ {
			if _, e := conn.ExecContext(ctx, "UPDATE schema_migration_steps SET inflight=TRUE,updated_at=UTC_TIMESTAMP(6) WHERE version=?", f.name); e != nil {
				return result, ErrStorage
			}
			if _, e := conn.ExecContext(ctx, f.statements[j]); e != nil {
				_, _ = conn.ExecContext(ctx, "UPDATE schema_migration_steps SET status='failed',updated_at=UTC_TIMESTAMP(6) WHERE version=?", f.name)
				return result, ErrUncertain
			}
			if _, e := conn.ExecContext(ctx, "UPDATE schema_migration_steps SET next_statement=?,inflight=FALSE,updated_at=UTC_TIMESTAMP(6) WHERE version=?", j+1, f.name); e != nil {
				return result, ErrUncertain
			}
		}
		tx, e := conn.BeginTx(ctx, nil)
		if e != nil {
			return result, ErrStorage
		}
		if _, e := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(?,?)", f.name, f.checksum); e != nil {
			tx.Rollback()
			return result, ErrStorage
		}
		if _, e := tx.ExecContext(ctx, "UPDATE schema_migration_steps SET status='applied',updated_at=UTC_TIMESTAMP(6) WHERE version=?", f.name); e != nil {
			tx.Rollback()
			return result, ErrStorage
		}
		if tx.Commit() != nil {
			return result, ErrUncertain
		}
		result.Applied++
	}
	return result, nil
}
