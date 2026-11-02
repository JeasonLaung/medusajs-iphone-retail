package catalogimport

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/rowimport"
)

// ImportProductReviewHistory retains source moderation and purchase facts, but
// never restores new-account authority. The complete verified snapshot and all
// parents must be staged in the same transaction. Roll back on every error.
func ImportProductReviewHistory(ctx context.Context, tx *sql.Tx, sha string, row rowimport.SourceRow) error {
	if tx == nil {
		return ErrPolicy
	}
	if _, err := rowimport.Convert("product_review", row); err != nil {
		return ErrPolicy
	}
	var rating int
	if json.Unmarshal(row["rating"].Raw, &rating) != nil || rating < 1 || rating > 5 {
		return ErrPolicy
	}
	var version int
	var reads, writes bool
	if tx.QueryRowContext(ctx, "SELECT version,reads_enabled,writes_enabled FROM product_review_runtime WHERE id=1 FOR UPDATE").Scan(&version, &reads, &writes) != nil || version != 1 || reads || writes {
		return ErrPolicy
	}
	id, err := importvalue.Text(row["id"].Raw, 191)
	if err != nil {
		return ErrPolicy
	}
	// account_id and command receipts are deliberately outside the source
	// projection. They must not silently pass an otherwise exact replay.
	var account sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT account_id FROM product_reviews WHERE id=? FOR UPDATE", id).Scan(&account)
	if err != nil && err != sql.ErrNoRows {
		return ErrStorage
	}
	if account.Valid {
		return ErrConflict
	}
	var commands int
	if tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM product_review_commands WHERE review_id=?", id).Scan(&commands) != nil {
		return ErrStorage
	}
	if commands != 0 {
		return ErrConflict
	}
	all, err := historyLayouts()
	if err != nil {
		return ErrPolicy
	}
	l, ok := all["product_review"]
	if !ok || l.Target != "product_reviews" {
		return ErrPolicy
	}
	return importReferencedHistory(ctx, tx, sha, "product_review", row, l, all)
}
