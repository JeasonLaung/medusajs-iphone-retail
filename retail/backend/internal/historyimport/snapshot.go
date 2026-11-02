// Package historyimport coordinates reviewed business history and inventory.
// It never restores login ownership, invokes providers or commits transactions.
package historyimport

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"

	"github.com/eurorefurb/retail/internal/catalogimport"
	"github.com/eurorefurb/retail/internal/customerimport"
	"github.com/eurorefurb/retail/internal/inventoryimport"
	"github.com/eurorefurb/retail/internal/orderimport"
	"github.com/eurorefurb/retail/internal/paymentimport"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
)

var ErrPolicy = errors.New("history snapshot requires reviewed policy")
var ErrConflict = errors.New("history snapshot target conflicts with source")

type Report struct {
	SnapshotSHA               string                       `json:"snapshot_sha256"`
	Catalog                   catalogimport.SnapshotReport `json:"catalog"`
	OrderRows                 int64                        `json:"order_rows"`
	CustomerRows              int64                        `json:"historical_customer_rows"`
	InventoryRows             int64                        `json:"inventory_rows"`
	PaymentSourceRows         int64                        `json:"payment_history_source_rows"`
	PaymentHistoryStaged      bool                         `json:"payment_history_staged"`
	PaymentReferenceHolds     int                          `json:"payment_reference_holds"`
	DeletedReservationMissing int64                        `json:"preserved_deleted_reservation_missing_line_items"`
	OrderReferenceHolds       int                          `json:"order_reference_holds"`
	InventoryHolds            int                          `json:"inventory_quantity_holds"`
	TargetVerified            bool                         `json:"target_verified"`
	Applied                   bool                         `json:"applied"`
	CompleteImportAccepted    bool                         `json:"complete_import_accepted"`
	Logistics                 *LogisticsReport             `json:"logistics,omitempty"`
	CartSourceRows            int64                        `json:"cart_history_source_rows"`
	Carts                     *CartReport                  `json:"carts,omitempty"`
	DevicePrerequisiteRows    int64                        `json:"device_prerequisite_source_rows"`
	DevicePrerequisites       *DevicePrerequisiteReport    `json:"device_prerequisites,omitempty"`
	ProductReviewSourceRows   int64                        `json:"product_review_source_rows"`
	ProductReviews            *ProductReviewReport         `json:"product_reviews,omitempty"`
}

type Snapshot struct {
	catalog             *catalogimport.CatalogSnapshot
	orders              map[string][]rowimport.SourceRow
	customers           []rowimport.SourceRow
	inventory           map[string][]rowimport.SourceRow
	payments            map[string][]rowimport.SourceRow
	fulfillment         map[string][]rowimport.SourceRow
	tax                 map[string][]rowimport.SourceRow
	carts               map[string][]rowimport.SourceRow
	devicePrerequisites map[string][]rowimport.SourceRow
	productReviews      []rowimport.SourceRow
	targets             map[string]string
	report              Report
}

// Prepare verifies the entire stream before any target connection. The sole
// inventory reference exception is the compiled deleted-reservation policy.
// Gaps remain reported; this exception never makes full migration accepted.
func Prepare(ctx context.Context, input io.ReadSeeker, sha string, maxRows, maxBytes int64) (*Snapshot, error) {
	if ctx.Err() != nil {
		return nil, ErrPolicy
	}
	catalog, err := catalogimport.PrepareCatalog(ctx, input, sha, maxRows, maxBytes)
	if err != nil {
		return nil, ErrPolicy
	}
	names, err := orderimport.SourceTables()
	if err != nil {
		return nil, ErrPolicy
	}
	p := &Snapshot{catalog: catalog, orders: map[string][]rowimport.SourceRow{}, inventory: map[string][]rowimport.SourceRow{"inventory_level": nil, "reservation_item": nil}, targets: map[string]string{}, report: Report{SnapshotSHA: sha, Catalog: catalog.Report()}}
	for _, name := range names {
		p.orders[name] = nil
	}
	p.payments = map[string][]rowimport.SourceRow{}
	names, err = paymentimport.SourceTables()
	if err != nil {
		return nil, ErrPolicy
	}
	for _, name := range names {
		p.payments[name] = nil
	}
	p.fulfillment, p.tax, err = logisticsTables()
	if err != nil {
		return nil, err
	}
	p.carts = map[string][]rowimport.SourceRow{}
	for _, name := range catalogimport.CartHistoryTables() {
		p.carts[name] = nil
	}
	p.devicePrerequisites = map[string][]rowimport.SourceRow{}
	for _, name := range catalogimport.DevicePrerequisiteTables() {
		p.devicePrerequisites[name] = nil
	}
	layouts, err := rowimport.Layouts()
	if err != nil {
		return nil, ErrPolicy
	}
	for _, l := range layouts {
		p.targets[l.Source] = l.Target
	}
	if _, err = input.Seek(0, io.SeekStart); err != nil {
		return nil, ErrPolicy
	}
	var total, retained int64
	_, err = rowimport.VerifyStream(input, sha, func(name string, row rowimport.SourceRow) error {
		if ctx.Err() != nil || total >= maxRows {
			return ErrPolicy
		}
		total++
		_, order := p.orders[name]
		_, stock := p.inventory[name]
		_, payment := p.payments[name]
		_, fulfillment := p.fulfillment[name]
		_, tax := p.tax[name]
		_, cart := p.carts[name]
		_, devicePrerequisite := p.devicePrerequisites[name]
		if !order && !stock && !payment && !fulfillment && !tax && !cart && !devicePrerequisite && name != "customer" && name != "product_review" {
			return nil
		}
		for k, c := range row {
			retained += int64(len(k) + len(c.Raw) + 1)
			if retained > maxBytes {
				return ErrPolicy
			}
		}
		if payment {
			v, e := rowimport.InspectValues(name, row)
			if e != nil || len(v.InvalidColumns) != 0 || !v.NumberPairsValid || !v.SourceChecksValid {
				return ErrPolicy
			}
			if v.OrdinaryConvertible {
				if _, e = rowimport.Convert(name, row); e != nil {
					return ErrPolicy
				}
			}
			p.payments[name] = append(p.payments[name], row)
			p.report.PaymentSourceRows++
			return nil
		}
		if name == "customer" {
			v, e := rowimport.InspectValues(name, row)
			if e != nil || len(v.InvalidColumns) != 0 || !v.NumberPairsValid || !v.SourceChecksValid {
				return ErrPolicy
			}
			if !row["metadata"].SQLNull {
				if _, e = rowimport.ReviewCustomerMetadata(row["metadata"].Raw); e != nil {
					return ErrPolicy
				}
			}
			p.customers = append(p.customers, row)
			p.report.CustomerRows++
			return nil
		}
		if _, e := rowimport.Convert(name, row); e != nil {
			return ErrPolicy
		}
		if order {
			p.orders[name] = append(p.orders[name], row)
			p.report.OrderRows++
		} else if stock {
			p.inventory[name] = append(p.inventory[name], row)
			p.report.InventoryRows++
		} else if fulfillment {
			p.fulfillment[name] = append(p.fulfillment[name], row)
		} else if tax {
			p.tax[name] = append(p.tax[name], row)
		} else if cart {
			p.carts[name] = append(p.carts[name], row)
			p.report.CartSourceRows++
		} else if devicePrerequisite {
			p.devicePrerequisites[name] = append(p.devicePrerequisites[name], row)
			p.report.DevicePrerequisiteRows++
		} else if name == "product_review" {
			p.productReviews = append(p.productReviews, row)
			p.report.ProductReviewSourceRows++
		}
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
	for key := range relations.Relations.Missing {
		if strings.SplitN(key, ".", 2)[0] == "product_review" {
			return nil, ErrPolicy
		}
		if _, included := p.devicePrerequisites[strings.SplitN(key, ".", 2)[0]]; included {
			return nil, ErrPolicy
		}
		if _, cart := p.carts[strings.SplitN(key, ".", 2)[0]]; cart {
			return nil, ErrPolicy
		}
	}
	p.report.DeletedReservationMissing, err = reviewInventoryReferences(relations.Relations)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func reviewInventoryReferences(r rowimport.RelationEvidence) (int64, error) {
	var preserved int64
	for key, n := range r.Missing {
		name := strings.SplitN(key, ".", 2)[0]
		if name != "inventory_level" && name != "reservation_item" {
			continue
		}
		if key != "reservation_item.line_item_id" || n <= 0 || r.HistoryPolicyMissing[key] != n || r.MissingOnDeletedRows[key] != n {
			return 0, ErrPolicy
		}
		preserved = n
	}
	if len(r.PhysicalMissing) != 0 {
		return 0, ErrPolicy
	}
	return preserved, nil
}

// Stage must run while all runtime write/worker gates are disabled, in the
// caller's transaction. Every error requires rollback, including later checks.
// Historical order dependencies unavailable in this scope receive the existing
// immutable order reference holds; inventory quantities receive quantity holds.
func (p *Snapshot) Stage(ctx context.Context, tx *sql.Tx, ring *protected.Ring) (Report, error) {
	return p.stage(ctx, tx, ring, nil)
}

// StageWithPayments additionally imports the eight reviewed historical payment
// tables with a separate persistent keyring. This preserves encrypted history
// and execution exclusions, never invokes Mollie or resumes old payment work.
func (p *Snapshot) StageWithPayments(ctx context.Context, tx *sql.Tx, ring, paymentRing *protected.Ring) (Report, error) {
	if paymentRing == nil {
		return Report{}, ErrPolicy
	}
	return p.stage(ctx, tx, ring, paymentRing)
}

func (p *Snapshot) stage(ctx context.Context, tx *sql.Tx, ring, paymentRing *protected.Ring) (Report, error) {
	if p == nil || tx == nil || ring == nil {
		return Report{}, ErrPolicy
	}
	r := p.Report()
	if err := p.catalog.Stage(ctx, tx); err != nil {
		return Report{}, err
	}
	counts := map[string]int{"historical_customer_subjects": len(p.customers)}
	for name, rows := range p.orders {
		counts[p.targets[name]] = len(rows)
	}
	for name, rows := range p.inventory {
		counts[p.targets[name]] = len(rows)
	}
	if paymentRing != nil {
		for name, rows := range p.payments {
			counts[p.targets[name]] = len(rows)
		}
	}
	for table, expected := range counts {
		var n int
		if tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&n) != nil {
			return Report{}, ErrPolicy
		}
		if n != 0 && n != expected {
			return Report{}, ErrConflict
		}
	}
	customers, err := customerimport.New(ring)
	if err != nil {
		return Report{}, err
	}
	for _, row := range p.customers {
		if err = customers.Import(ctx, tx, p.report.SnapshotSHA, row); err != nil {
			return Report{}, err
		}
	}
	if paymentRing != nil {
		payments, e := paymentimport.New(paymentRing)
		if e != nil {
			return Report{}, e
		}
		result, e := payments.Import(ctx, tx, p.report.SnapshotSHA, p.payments)
		if e != nil {
			return Report{}, e
		}
		r.PaymentHistoryStaged = true
		r.PaymentReferenceHolds = result.ObservedMissingReferences
	}
	orders, err := orderimport.Import(ctx, tx, p.report.SnapshotSHA, p.orders)
	if err != nil {
		return Report{}, err
	}
	r.OrderReferenceHolds = orders.ObservedMissingReferences
	for _, name := range []string{"inventory_level", "reservation_item"} {
		for _, row := range p.inventory[name] {
			if err = inventoryimport.Import(ctx, tx, p.report.SnapshotSHA, name, row); err != nil {
				return Report{}, err
			}
		}
	}
	r.InventoryHolds, err = inventoryimport.PreserveHolds(ctx, tx, p.report.SnapshotSHA)
	if err != nil {
		return Report{}, err
	}
	r.TargetVerified = true
	return r, nil
}

func (p *Snapshot) Report() Report {
	r := p.report
	r.Catalog = p.catalog.Report()
	return r
}
