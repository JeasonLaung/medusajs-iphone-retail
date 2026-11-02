package historyimport

import (
	"context"
	"database/sql"

	"github.com/eurorefurb/retail/internal/fulfillmentimport"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/internal/taximport"
)

type LogisticsReport struct {
	FulfillmentRows           int `json:"fulfillment_rows"`
	FulfillmentReferenceHolds int `json:"fulfillment_reference_holds"`
	TaxRows                   int `json:"tax_rows"`
	TaxReferenceHolds         int `json:"tax_reference_holds"`
}

func logisticsTables() (map[string][]rowimport.SourceRow, map[string][]rowimport.SourceRow, error) {
	fulfillment, err := fulfillmentimport.SourceTables()
	if err != nil {
		return nil, nil, ErrPolicy
	}
	tax, err := taximport.SourceTables()
	if err != nil {
		return nil, nil, ErrPolicy
	}
	f := map[string][]rowimport.SourceRow{}
	t := map[string][]rowimport.SourceRow{}
	for _, name := range fulfillment {
		f[name] = nil
	}
	for _, name := range tax {
		t[name] = nil
	}
	return f, t, nil
}

// StageLogistics follows Stage in the SAME caller transaction. It preserves
// historical fulfillment and tax facts with their package-owned ledgers/holds;
// it neither ships orders nor invokes carriers or tax providers. Any error,
// including a later failure, requires rolling back the entire transaction.
func (p *Snapshot) StageLogistics(ctx context.Context, tx *sql.Tx) (LogisticsReport, error) {
	if p == nil || tx == nil || len(p.fulfillment) != 8 || len(p.tax) != 4 {
		return LogisticsReport{}, ErrPolicy
	}
	for _, group := range []map[string][]rowimport.SourceRow{p.fulfillment, p.tax} {
		for name, rows := range group {
			var count int
			if tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+p.targets[name]+"`").Scan(&count) != nil {
				return LogisticsReport{}, ErrPolicy
			}
			if count != 0 && count != len(rows) {
				return LogisticsReport{}, ErrConflict
			}
		}
	}
	f, err := fulfillmentimport.Import(ctx, tx, p.report.SnapshotSHA, p.fulfillment)
	if err != nil {
		return LogisticsReport{}, err
	}
	t, err := taximport.Import(ctx, tx, p.report.SnapshotSHA, p.tax)
	if err != nil {
		return LogisticsReport{}, err
	}
	return LogisticsReport{FulfillmentRows: f.Rows, FulfillmentReferenceHolds: f.ObservedMissingReferences, TaxRows: t.Rows, TaxReferenceHolds: t.ObservedMissingReferences}, nil
}
