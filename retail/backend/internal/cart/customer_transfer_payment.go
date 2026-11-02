package cart

import (
	"context"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Authenticate the immutable original checkout and every live unpaid source
// before temporarily attaching a customer and recomputing qualified rules.
// Caller owns the owner lock and the entire transaction. No provider calls.
func validateCustomerTransferPayment(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, current Snapshot, checkout string, options InitialPaymentOptions) error {
	if err := initialOptions(options); err != nil {
		return err
	}
	if err := validateContextPaymentCheckout(ctx, tx, ref, current, checkout, options); err != nil {
		return err
	}
	var attempt initialAttempt
	if initialWhere(tx, ref, InitialPaymentCommand{OperationID: checkout}).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&attempt).Error != nil {
		return ErrStorage
	}
	plan := PaymentRefreshPlan{CollectionID: attempt.CollectionID, UpdateCollection: true}
	if tx.Table("payment_sessions").Select("id").Where("payment_collection_id=? AND deleted_at IS NULL", plan.CollectionID).Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(101).Scan(&plan.DeleteSessionIDs).Error != nil || len(plan.DeleteSessionIDs) != 1 || plan.DeleteSessionIDs[0] != attempt.SessionID {
		return ErrConflict
	}
	_, sources, _, err := paymentSources(ctx, tx, current.ID, plan)
	if err != nil {
		return err
	}
	for _, source := range sources {
		n, err := nonNegativeAmount(source.Amount, &source.RawAmount)
		if err != nil {
			return err
		}
		amount, err := totals.Resolve(n)
		if err != nil {
			return err
		}
		request := payments.SessionRequest{ProviderID: source.ProviderID, CollectionID: source.PaymentCollectionID, SessionID: source.ID, Amount: amount, Currency: source.CurrencyCode}
		if _, err := payments.OpenSessionFields(options.Ring, request, source.Data, source.Context); err != nil {
			return err
		}
	}
	return nil
}
