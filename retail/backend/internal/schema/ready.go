package schema

import (
	"context"
	"database/sql"
)

// CheckApplied is read-only startup verification. It requires the complete
// compiled plan and its statement journal, not merely any migration row.
func CheckApplied(ctx context.Context, db *sql.DB, p *Plan) error {
	if db == nil || p == nil || len(p.files) == 0 {
		return ErrPlan
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return ErrStorage
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT m.version,m.checksum,s.checksum,s.statement_count,s.next_statement,s.inflight,s.status FROM schema_migrations m LEFT JOIN schema_migration_steps s ON s.version=m.version ORDER BY m.version`)
	if err != nil {
		return ErrStorage
	}
	index := 0
	for rows.Next() {
		var name, checksum string
		var stepChecksum, status sql.NullString
		var count, next sql.NullInt64
		var inflight sql.NullBool
		if rows.Scan(&name, &checksum, &stepChecksum, &count, &next, &inflight, &status) != nil {
			rows.Close()
			return ErrDrift
		}
		if index >= len(p.files) {
			rows.Close()
			return ErrDrift
		}
		f := p.files[index]
		index++
		if name != f.name || checksum != f.checksum || !stepChecksum.Valid || stepChecksum.String != f.checksum || !count.Valid || count.Int64 != int64(len(f.statements)) || !next.Valid || next.Int64 != count.Int64 || !inflight.Valid || inflight.Bool || !status.Valid || status.String != "applied" {
			rows.Close()
			return ErrDrift
		}
	}
	if rows.Err() != nil || rows.Close() != nil {
		return ErrStorage
	}
	if index != len(p.files) {
		return ErrDrift
	}
	var steps int
	if tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migration_steps").Scan(&steps) != nil {
		return ErrStorage
	}
	if steps != len(p.files) {
		return ErrDrift
	}
	if tx.Commit() != nil {
		return ErrStorage
	}
	return nil
}
