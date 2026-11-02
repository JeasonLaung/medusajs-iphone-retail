package historyimport

import (
	"context"
	"database/sql"

	"github.com/eurorefurb/retail/internal/catalogimport"
)

type ProductReviewReport struct {
	Rows               int  `json:"rows"`
	NewAccountBindings int  `json:"new_account_bindings"`
	RuntimeEnabled     bool `json:"runtime_enabled"`
}

// StageProductReviews follows the base customer/order/catalog stage in the same
// transaction. Missing parents, changed moderation, runtime authority and partial
// target populations are conflicts, never repaired by overwriting source facts.
func (p *Snapshot) StageProductReviews(ctx context.Context, tx *sql.Tx) (ProductReviewReport, error) {
	if p == nil || tx == nil {
		return ProductReviewReport{}, ErrPolicy
	}
	var version int
	var reads, writes bool
	if tx.QueryRowContext(ctx, "SELECT version,reads_enabled,writes_enabled FROM product_review_runtime WHERE id=1 FOR UPDATE").Scan(&version, &reads, &writes) != nil || version != 1 || reads || writes {
		return ProductReviewReport{}, ErrPolicy
	}
	var count, bindings, commands int
	if tx.QueryRowContext(ctx, "SELECT COUNT(*),COUNT(account_id) FROM product_reviews").Scan(&count, &bindings) != nil || tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM product_review_commands").Scan(&commands) != nil {
		return ProductReviewReport{}, ErrPolicy
	}
	if bindings != 0 || commands != 0 || count != 0 && count != len(p.productReviews) {
		return ProductReviewReport{}, ErrConflict
	}
	for _, row := range p.productReviews {
		if err := catalogimport.ImportProductReviewHistory(ctx, tx, p.report.SnapshotSHA, row); err != nil {
			return ProductReviewReport{}, err
		}
	}
	return ProductReviewReport{Rows: len(p.productReviews)}, nil
}
