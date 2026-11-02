package catalogimport

import (
	"context"
	"database/sql"
	"sync"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/rowimport"
)

// Historical cart facts reuse the immutable row evidence writer, as buyback
// history does. They do not expand the 37-table catalog generator or restore
// any possession, creation scope, account binding or payment attempt.
var cartHistoryOrder = []string{"cart_address", "cart", "cart_line_item", "cart_line_item_adjustment", "cart_line_item_tax_line", "cart_payment_collection", "cart_promotion", "cart_shipping_method", "cart_shipping_method_adjustment", "cart_shipping_method_tax_line"}

func CartHistoryTables() []string { return append([]string(nil), cartHistoryOrder...) }

var historyLayouts = sync.OnceValues(func() (map[string]rowimport.Layout, error) {
	layouts, err := rowimport.Layouts()
	if err != nil {
		return nil, ErrPolicy
	}
	all := map[string]rowimport.Layout{}
	for _, l := range layouts {
		all[l.Source] = l
	}
	for _, name := range cartHistoryOrder {
		l, ok := all[name]
		if !ok || len(l.Primary) == 0 {
			return nil, ErrPolicy
		}
	}
	return all, nil
})

// ImportCartHistory requires the caller to verify the full v2 snapshot and
// stage all referenced domains before this row. Every reference is checked
// against compiled mappings, including soft-deleted parents. Missing parents
// are not fabricated. Roll back the caller transaction on ANY error.
func ImportCartHistory(ctx context.Context, tx *sql.Tx, sha, name string, row rowimport.SourceRow) error {
	if tx == nil {
		return ErrPolicy
	}
	allowed := false
	for _, n := range cartHistoryOrder {
		if n == name {
			allowed = true
			break
		}
	}
	if !allowed {
		return ErrPolicy
	}
	all, err := historyLayouts()
	if err != nil {
		return err
	}
	l := all[name]
	if _, err = rowimport.Convert(name, row); err != nil {
		return ErrPolicy
	}
	if name == "cart" {
		id, err := importvalue.Text(row["id"].Raw, 191)
		if err != nil {
			return ErrPolicy
		}
		var owned int
		if tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM cart_ownership WHERE cart_id=?", id).Scan(&owned) != nil {
			return ErrStorage
		}
		if owned != 0 {
			return ErrConflict
		}
	}
	return importReferencedHistory(ctx, tx, sha, name, row, l, all)
}

// importReferencedHistory only accepts layouts selected by the fixed domain entry points.
// It checks every parent before using the common immutable evidence writer.
func importReferencedHistory(ctx context.Context, tx *sql.Tx, sha, name string, row rowimport.SourceRow, l rowimport.Layout, all map[string]rowimport.Layout) error {
	for _, ref := range l.References {
		cell := row[ref.Column]
		if cell.SQLNull {
			continue
		}
		value, err := importvalue.Text(cell.Raw, 191)
		if err != nil {
			return ErrPolicy
		}
		parent, ok := all[ref.Parent]
		if !ok {
			return ErrPolicy
		}
		column := ""
		for _, c := range parent.Columns {
			if c.Source == ref.ParentColumn {
				column = c.Target
				break
			}
		}
		if column == "" {
			return ErrPolicy
		}
		var found int
		err = tx.QueryRowContext(ctx, "SELECT 1 FROM `"+parent.Target+"` WHERE `"+column+"`=? LIMIT 1 FOR SHARE", value).Scan(&found)
		if err == sql.ErrNoRows {
			return ErrConflict
		}
		if err != nil {
			return ErrStorage
		}
	}
	t := table{Source: name, Target: l.Target}
	columns := map[string]column{}
	for _, c := range l.Columns {
		v := column{Source: c.Source, Target: c.Target, Decimal: c.Decimal}
		t.Columns = append(t.Columns, v)
		columns[c.Source] = v
	}
	for _, pk := range l.Primary {
		c, ok := columns[pk]
		if !ok {
			return ErrPolicy
		}
		t.Primary = append(t.Primary, c)
	}
	return importTable(ctx, tx, sha, name, row, t)
}
