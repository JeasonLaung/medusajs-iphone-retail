package historyimport

import (
	"context"
	"database/sql"

	"github.com/eurorefurb/retail/internal/catalogimport"
)

type CartReport struct {
	Rows                     int  `json:"rows"`
	RuntimeAuthorityRestored bool `json:"runtime_authority_restored"`
}

// StageCarts follows the base history stage in the SAME transaction. Payment
// parents must already exist (e.g. StageWithPayments). No authority or command
// tables are written, and no missing reference is accepted. Any error requires
// rolling back the complete caller transaction.
func (p *Snapshot) StageCarts(ctx context.Context, tx *sql.Tx) (CartReport, error) {
	if p == nil || tx == nil || len(p.carts) != 10 {
		return CartReport{}, ErrPolicy
	}
	var version int
	var reads, writes bool
	if tx.QueryRowContext(ctx, "SELECT schema_version,reads_enabled,writes_enabled FROM cart_runtime_schema WHERE id=1 FOR UPDATE").Scan(&version, &reads, &writes) != nil || version != 1 || !reads || writes {
		return CartReport{}, ErrPolicy
	}
	capabilities, err := tx.QueryContext(ctx, "SELECT command,version,enabled FROM cart_command_capabilities ORDER BY command FOR UPDATE")
	if err != nil {
		return CartReport{}, ErrPolicy
	}
	seen := map[string]bool{}
	for capabilities.Next() {
		var name string
		var v int
		var enabled bool
		if capabilities.Scan(&name, &v, &enabled) != nil || (name != "cart.create" && name != "cart.access") || v != 1 || enabled {
			capabilities.Close()
			return CartReport{}, ErrPolicy
		}
		seen[name] = true
	}
	if capabilities.Err() != nil || capabilities.Close() != nil || len(seen) != 2 {
		return CartReport{}, ErrPolicy
	}
	var result CartReport
	for _, name := range catalogimport.CartHistoryTables() {
		rows := p.carts[name]
		var count int
		if tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+p.targets[name]+"`").Scan(&count) != nil {
			return CartReport{}, ErrPolicy
		}
		if count != 0 && count != len(rows) {
			return CartReport{}, ErrConflict
		}
		for _, row := range rows {
			if err := catalogimport.ImportCartHistory(ctx, tx, p.report.SnapshotSHA, name, row); err != nil {
				return CartReport{}, err
			}
			result.Rows++
		}
	}
	return result, nil
}
