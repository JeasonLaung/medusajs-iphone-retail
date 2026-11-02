package catalogimport

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/rowimport"
)

// SnapshotReport describes only the public catalog dependency closure. Missing
// references outside it remain unresolved and are never treated as accepted.
type SnapshotReport struct {
	SnapshotSHA            string           `json:"snapshot_sha256"`
	SourceRows             int64            `json:"source_rows"`
	Tables                 int              `json:"catalog_and_dependency_tables"`
	Rows                   int64            `json:"catalog_and_dependency_rows"`
	OutsideMissing         map[string]int64 `json:"unresolved_outside_scope"`
	Applied                bool             `json:"applied"`
	CompleteImportAccepted bool             `json:"complete_import_accepted"`
}

type CatalogSnapshot struct {
	report  SnapshotReport
	order   []string
	layouts map[string]rowimport.Layout
	rows    map[string][]rowimport.SourceRow
	parents map[string]table
}

// PrepareCatalog verifies the entire v2 stream before retaining an importable
// dependency closure. It does no SQL, account binding, files or side effects.
// Limits bound both row count and retained source-cell payload (not heap).
func PrepareCatalog(ctx context.Context, input io.ReadSeeker, sha string, maxRows, maxBytes int64) (*CatalogSnapshot, error) {
	if input == nil || maxRows < 1 || maxBytes < 1 {
		return nil, ErrPolicy
	}
	catalog, err := plans()
	if err != nil {
		return nil, err
	}
	layouts, err := rowimport.Layouts()
	if err != nil {
		return nil, ErrPolicy
	}
	p := &CatalogSnapshot{layouts: map[string]rowimport.Layout{}, rows: map[string][]rowimport.SourceRow{}, parents: map[string]table{}}
	for _, l := range layouts {
		p.layouts[l.Source] = l
	}
	scope := map[string]bool{}
	for name := range catalog {
		scope[name] = true
	}
	for changed := true; changed; {
		changed = false
		for name := range scope {
			for _, r := range p.layouts[name].References {
				if !scope[r.Parent] {
					scope[r.Parent] = true
					changed = true
				}
			}
		}
	}
	// These nine ordinary parent domains have no protected/account fields.
	// Expanding this policy requires explicit review, never a caller allowlist.
	parentNames := map[string]bool{"fulfillment_provider": true, "fulfillment_set": true, "inventory_item": true, "payment_provider": true, "service_zone": true, "shipping_option": true, "shipping_option_type": true, "stock_location": true, "stock_location_address": true}
	if len(scope) != 46 || len(catalog) != 37 {
		return nil, ErrPolicy
	}
	for name := range scope {
		l, ok := p.layouts[name]
		if !ok {
			return nil, ErrPolicy
		}
		p.rows[name] = nil
		if _, ordinary := catalog[name]; ordinary {
			continue
		}
		if !parentNames[name] {
			return nil, ErrPolicy
		}
		t := table{Source: name, Target: l.Target}
		for _, c := range l.Columns {
			v := column{Source: c.Source, Target: c.Target, Decimal: c.Decimal}
			t.Columns = append(t.Columns, v)
			for _, pk := range l.Primary {
				if pk == c.Source {
					t.Primary = append(t.Primary, v)
				}
			}
		}
		if len(t.Primary) != 1 {
			return nil, ErrPolicy
		}
		p.parents[name] = t
	}
	if _, err = input.Seek(0, io.SeekStart); err != nil {
		return nil, ErrPolicy
	}
	var total, retained int64
	verified, err := rowimport.VerifyStream(input, sha, func(name string, row rowimport.SourceRow) error {
		if ctx.Err() != nil || total >= maxRows {
			return ErrPolicy
		}
		total++
		if !scope[name] {
			return nil
		}
		if _, err := rowimport.Convert(name, row); err != nil {
			return ErrPolicy
		}
		for k, c := range row {
			retained += int64(len(k) + len(c.Raw) + 1)
			if retained > maxBytes {
				return ErrPolicy
			}
		}
		p.rows[name] = append(p.rows[name], row)
		p.report.Rows++
		return nil
	})
	if err != nil {
		return nil, ErrPolicy
	}
	if _, err = input.Seek(0, io.SeekStart); err != nil {
		return nil, ErrPolicy
	}
	relations, err := rowimport.VerifyRelationships(input, sha, rowimport.RelationLimits{MaxRows: maxRows, KeyBytes: maxBytes})
	if err != nil && !errors.Is(err, rowimport.ErrReferences) {
		return nil, ErrPolicy
	}
	if relations.Snapshot.SHA256 != verified.SHA256 {
		return nil, ErrPolicy
	}
	for key := range relations.Relations.Missing {
		if scope[strings.SplitN(key, ".", 2)[0]] {
			return nil, ErrPolicy
		}
	}
	if err = p.sort(scope); err != nil {
		return nil, err
	}
	p.report.SnapshotSHA = sha
	p.report.SourceRows = total
	p.report.Tables = len(scope)
	p.report.OutsideMissing = relations.Relations.Missing
	return p, nil
}

func (p *CatalogSnapshot) sort(scope map[string]bool) error {
	pending := map[string]bool{}
	for name := range scope {
		pending[name] = true
	}
	for len(pending) > 0 {
		var ready []string
		for name := range pending {
			blocked := false
			for _, r := range p.layouts[name].References {
				if r.Parent != name && pending[r.Parent] {
					blocked = true
				}
			}
			if !blocked {
				ready = append(ready, name)
			}
		}
		if len(ready) == 0 {
			return ErrPolicy
		}
		sort.Strings(ready)
		for _, name := range ready {
			p.order = append(p.order, name)
			delete(pending, name)
		}
	}
	for _, name := range p.order {
		for _, r := range p.layouts[name].References {
			if r.Parent != name {
				continue
			}
			byID := map[string]rowimport.SourceRow{}
			for _, row := range p.rows[name] {
				id, err := importvalue.Text(row[r.ParentColumn].Raw, 191)
				if err != nil || id == "" {
					return ErrPolicy
				}
				if _, ok := byID[id]; ok {
					return ErrPolicy
				}
				byID[id] = row
			}
			state := map[string]byte{}
			var sorted []rowimport.SourceRow
			var visit func(string) error
			visit = func(id string) error {
				if state[id] == 2 {
					return nil
				}
				if state[id] == 1 {
					return ErrPolicy
				}
				row, ok := byID[id]
				if !ok {
					return ErrPolicy
				}
				state[id] = 1
				if !row[r.Column].SQLNull {
					parent, err := importvalue.Text(row[r.Column].Raw, 191)
					if err != nil {
						return ErrPolicy
					}
					if err = visit(parent); err != nil {
						return err
					}
				}
				state[id] = 2
				sorted = append(sorted, row)
				return nil
			}
			ids := make([]string, 0, len(byID))
			for id := range byID {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				if err := visit(id); err != nil {
					return err
				}
			}
			p.rows[name] = sorted
		}
	}
	return nil
}

func (p *CatalogSnapshot) Report() SnapshotReport {
	r := p.report
	r.OutsideMissing = map[string]int64{}
	for k, v := range p.report.OutsideMissing {
		r.OutsideMissing[k] = v
	}
	return r
}

// Stage participates in the caller's transaction. Any failure MUST roll back.
// Every parent gets the same immutable source/target evidence as catalog rows.
// Existing changes conflict; there is no upsert of business facts or deletion.
func (p *CatalogSnapshot) Stage(ctx context.Context, tx *sql.Tx) error {
	if p == nil || tx == nil || len(p.order) != 46 {
		return ErrPolicy
	}
	var constraints int
	if tx.QueryRowContext(ctx, "SELECT @@foreign_key_checks").Scan(&constraints) != nil || constraints != 1 {
		return ErrPolicy
	}
	for _, name := range p.order {
		var count int64
		if tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+p.layouts[name].Target+"`").Scan(&count) != nil {
			return ErrStorage
		}
		if count != 0 && count != int64(len(p.rows[name])) {
			return ErrConflict
		}
		for _, row := range p.rows[name] {
			var err error
			if parent, ok := p.parents[name]; ok {
				err = importTable(ctx, tx, p.report.SnapshotSHA, name, row, parent)
			} else {
				err = Import(ctx, tx, p.report.SnapshotSHA, name, row)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}
