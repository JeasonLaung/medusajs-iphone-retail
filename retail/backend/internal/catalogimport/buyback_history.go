package catalogimport

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/rowimport"
)

// ImportBuybackHistory stages original requests, photo metadata and device
// links in the caller's isolated-target transaction. It never commits, grants
// login ownership, sends notifications, moves inventory or touches files.
// Parents must already be staged. Roll back on ANY error; commit only after
// full snapshot, reference and file reconciliation. External photo objects
// remain unresolved until the separate private-file migration has succeeded.
// This entry point deliberately does not expand the catalog schema generator.
func ImportBuybackHistory(ctx context.Context, tx *sql.Tx, snapshotSHA, sourceTable string, source rowimport.SourceRow) error {
	if tx == nil {
		return ErrPolicy
	}
	var target string
	switch sourceTable {
	case "buyback_request":
		target = "buyback_requests"
	case "buyback_photo":
		target = "buyback_photos"
	case "buyback_device_link":
		target = "buyback_device_links"
	default:
		return ErrPolicy
	}
	bound, err := rowimport.Convert(sourceTable, source)
	if err != nil || bound.Table != target {
		return ErrPolicy
	}
	// Layout names come exclusively from the compiled source field program.
	layout := table{Source: sourceTable, Target: target, Primary: []column{{Source: "id", Target: "id"}}, Registry: sourceTable == "buyback_request"}
	for _, name := range bound.Columns {
		layout.Columns = append(layout.Columns, column{Source: name, Target: name, Decimal: name == "reference_amount" || name == "final_amount"})
	}
	if sourceTable != "buyback_request" {
		request, err := importvalue.Text(source["request_id"].Raw, 191)
		if err != nil || request == "" {
			return ErrPolicy
		}
		item, err := importvalue.Text(source["item_id"].Raw, 191)
		if err != nil || item == "" {
			return ErrPolicy
		}
		// Item IDs live in request JSON; SQL foreign keys cannot prove this
		// relationship. Retain soft-deleted parents and children as history.
		var raw string
		if err := tx.QueryRowContext(ctx, "SELECT items FROM buyback_requests WHERE id=? FOR UPDATE", request).Scan(&raw); err != nil {
			return ErrStorage
		}
		if !buybackHasItem(raw, item) {
			return ErrConflict
		}
	}
	return importTable(ctx, tx, snapshotSHA, sourceTable, source, layout)
}

func buybackHasItem(raw, wanted string) bool {
	var items []struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(raw), &items) != nil {
		return false
	}
	seen := make(map[string]bool, len(items))
	found := false
	for _, item := range items {
		if item.ID == "" || seen[item.ID] {
			return false
		}
		seen[item.ID] = true
		found = found || item.ID == wanted
	}
	return found
}
