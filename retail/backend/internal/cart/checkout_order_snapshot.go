package cart

import (
	"context"
	"crypto/subtle"
	"encoding/json"

	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StagePaidOrderSnapshot is an INTERNAL participant in the owning checkout
// completion transaction. No HTTP route invokes it yet. The owner MUST check
// its completion receipt before calling, then reserve stock, register promotion
// usage, record cash, complete cart and save receipt/outbox in the SAME transaction.
// Any failure requires whole-transaction rollback; a staged snapshot alone must
// never be committed or reported as checkout success. No provider calls occur.
func StagePaidOrderSnapshot(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in InitialPaymentCommand, o InitialPaymentOptions) (orders.CheckoutSnapshot, error) {
	if tx == nil || tx.Statement == nil {
		return orders.CheckoutSnapshot{}, ErrInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return orders.CheckoutSnapshot{}, ErrInput
	}
	if err := initialOptions(o); err != nil {
		return orders.CheckoutSnapshot{}, err
	}
	row, key, err := stageInitialPayment(ctx, tx, store, ref, in, o)
	if err != nil {
		return orders.CheckoutSnapshot{}, err
	}
	if row.NeedsReview || row.ObservedStatus == nil || *row.ObservedStatus != "paid" {
		return orders.CheckoutSnapshot{}, ErrConflict
	}
	var proof struct {
		ID                            string
		RequestSHA256, ResultEnvelope []byte
	}
	r := tx.Table("cart_checkout_observations p").Select("p.id,p.request_sha256,p.result_envelope").Joins("JOIN cart_checkout_attempts a ON a.paid_observation_id=p.id AND a.cart_id=p.cart_id AND a.operation_id=p.operation_id").Where("a.cart_id=? AND a.operation_id=?", ref.CartID, in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&proof)
	if r.Error != nil || subtle.ConstantTimeCompare(proof.RequestSHA256, key.Digest[:]) != 1 {
		return orders.CheckoutSnapshot{}, ErrConflict
	}
	plain, err := o.Ring.Open(observationContext(proof.ID), proof.ResultEnvelope)
	if err != nil {
		return orders.CheckoutSnapshot{}, ErrStorage
	}
	defer clear(plain)
	var observed payments.SessionObservation
	if json.Unmarshal(plain, &observed) != nil || observed.ProviderStatus != "paid" {
		return orders.CheckoutSnapshot{}, ErrConflict
	}
	request, err := payments.ReadSessionRequest(ctx, tx, key, o.Ring)
	if err != nil {
		return orders.CheckoutSnapshot{}, err
	}
	created, err := payments.ReadSessionCreation(ctx, tx, key, o.Ring)
	if err != nil {
		return orders.CheckoutSnapshot{}, err
	}
	var identity struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(created.Data, &identity) != nil || identity.ID == "" || payments.ValidateMollieObservation(request, identity.ID, observed) != nil {
		return orders.CheckoutSnapshot{}, ErrConflict
	}
	channel, err := storeChannel(ctx, tx, store)
	if err != nil {
		return orders.CheckoutSnapshot{}, err
	}
	current, err := lockCurrentAuthority(ctx, tx, store, channel, ref, "UPDATE", "", in.OperationID)
	if err != nil {
		return orders.CheckoutSnapshot{}, err
	}
	amounts, err := assembleAmounts(tx, current)
	if err != nil || amounts.Totals["total"] != row.CartAmount {
		return orders.CheckoutSnapshot{}, ErrConflict
	}
	return orders.WriteCartSnapshot(ctx, tx, orders.CartSnapshotInput{CartID: ref.CartID, CurrencyCode: row.CurrencyCode, Totals: amounts.Totals})
}
