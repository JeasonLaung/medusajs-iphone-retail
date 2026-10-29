package cart

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This private capability exists only during the shipping READ below, after
// StagePaidOrderSnapshot has reauthorized frozen checkout and its sealed paid
// evidence. It cannot be supplied via HTTP or exported to another package. Bind
// the exact SQL transaction, cart reference and version; do not unfreeze the cart
// or pass this context to mutation/provider code. Nested quote/profile readers
// still perform ordinary durable authority/version checks on every access.
type checkoutReadScopeKey struct{}
type checkoutReadScope struct {
	tx                        gorm.ConnPool
	store, version, operation string
	ref                       sessionstore.CartReference
}

type PaidOrderStock struct {
	Snapshot       orders.CheckoutSnapshot
	ReservationIDs []string
}

// StagePaidOrderAndStock is INTERNAL, not a completion endpoint. The owner must
// check its completion receipt FIRST, invoke this participant, then register
// promotions, verified cash, completion receipt and notification outbox before
// committing the SAME transaction. Any error must roll back the whole order and
// all reservations. No provider call, commit or client success happens here.
func StagePaidOrderAndStock(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in InitialPaymentCommand, o InitialPaymentOptions) (PaidOrderStock, error) {
	// Snapshot first: its allocator lock also fixes a consistent order of locks
	// for concurrent new-order transactions before inventory planning/reserving.
	snapshot, err := StagePaidOrderSnapshot(ctx, tx, store, ref, in, o)
	if err != nil {
		return PaidOrderStock{}, err
	}
	sqlTx, ok := tx.Statement.ConnPool.(*sql.Tx)
	if !ok {
		return PaidOrderStock{}, ErrInput
	}
	var attempt initialAttempt
	if initialWhere(tx, ref, in).Clauses(clause.Locking{Strength: "SHARE"}).Take(&attempt).Error != nil {
		return PaidOrderStock{}, ErrStorage
	}
	version := strconv.FormatUint(attempt.CartVersion, 10)
	readCtx := context.WithValue(ctx, checkoutReadScopeKey{}, checkoutReadScope{tx: sqlTx, store: store, version: version, operation: in.OperationID, ref: ref})
	var selected []string
	if tx.Table("cart_shipping_methods").Select("shipping_option_id").Where("cart_id=? AND deleted_at IS NULL AND shipping_option_id IS NOT NULL", ref.CartID).Clauses(clause.Locking{Strength: "SHARE"}).Order("id").Limit(101).Scan(&selected).Error != nil || len(selected) > 100 {
		return PaidOrderStock{}, ErrSchema
	}
	if err = supportedSavedShipping(tx, ref.CartID, o.ManualProviders); err != nil {
		return PaidOrderStock{}, err
	}
	selection, err := ReadSelectedManualShipping(readCtx, tx, store, ref, version, selected, o.ManualProviders, time.Now().UTC())
	if err != nil {
		return PaidOrderStock{}, err
	}
	if !selection.Complete || !selection.Stock.CanFulfill {
		return PaidOrderStock{}, ErrItemStock
	}
	out := PaidOrderStock{Snapshot: snapshot, ReservationIDs: []string{}}
	if len(selection.Stock.Assignments) == 0 {
		return out, nil
	} // genuinely untracked/digital lines
	reserve := inventory.ReservationCommand{OperationID: in.OperationID, OwnerID: snapshot.OrderID, ActorRef: "checkout-paid-order", Kind: "reserve"}
	for _, assignment := range selection.Stock.Assignments {
		line, ok := snapshot.LineIDs[assignment.LineItemID]
		if !ok || line == "" {
			return PaidOrderStock{}, ErrConflict
		}
		reserve.Reserve = append(reserve.Reserve, inventory.ReservationInput{ID: "resitem_" + uuid.NewString(), InventoryItemID: assignment.InventoryItemID, LocationID: assignment.LocationID, LineItemID: line, Quantity: assignment.Quantity, AllowBackorder: assignment.AllowBackorder})
	}
	// ApplyReservations locks every selected item/location in stable order,
	// rechecks available stock, migration holds and sum of live reservations,
	// then updates reservations/level counters/operation receipt atomically.
	result, err := inventory.ApplyReservations(ctx, sqlTx, reserve)
	if err != nil {
		return PaidOrderStock{}, err
	}
	out.ReservationIDs = result.IDs
	return out, nil
}
