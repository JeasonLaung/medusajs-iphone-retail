package refunds

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type supplementalTrace struct {
	ScopeID, OperationID, SessionID, RemoteID, Amount            string
	RequestSHA256                                                []byte
	Data                                                         []byte
	PaymentAmount, PaymentRaw, TransactionAmount, TransactionRaw string
}

// Applied cash keeps its provenance independently of current customer authority
// and retired anonymous-cart capabilities. It never falls back to matching email.
func (s Service) supplementalTrace(tx *gorm.DB, order, payment, collection, session, provider string) (supplementalTrace, error) {
	var trace supplementalTrace
	e := tx.Table("order_payment_operations o").Select("o.order_id AS scope_id,o.operation_id,o.session_id,r.remote_id,o.request_sha256,o.amount,p.data,CAST(p.amount AS CHAR) AS payment_amount,p.raw_amount AS payment_raw,CAST(x.amount AS CHAR) AS transaction_amount,x.raw_amount AS transaction_raw").
		Joins("JOIN cart_completion_receipts origin ON origin.order_id=o.order_id AND origin.cart_id=o.cart_id").
		Joins("JOIN payments p ON p.id=CONCAT('pay_supp_',o.operation_id) AND p.payment_collection_id=o.collection_id AND p.payment_session_id=o.session_id AND p.provider_id=o.provider_id AND p.currency_code=o.currency_code AND p.captured_at IS NOT NULL AND p.canceled_at IS NULL AND p.deleted_at IS NULL").
		Joins("JOIN payment_captures c ON c.id=CONCAT('cap_supp_',o.operation_id) AND c.payment_id=p.id AND c.deleted_at IS NULL").
		Joins("JOIN order_transactions x ON x.id=CONCAT('ordtr_supp_',o.operation_id) AND x.order_id=o.order_id AND x.reference='capture' AND x.reference_id=c.id AND x.deleted_at IS NULL").
		Joins("JOIN mollie_payment_references r ON r.scope_id=o.order_id AND r.operation_id=o.operation_id AND r.resource_id=o.session_id AND r.request_sha256=o.request_sha256 AND r.provider_id=o.provider_id AND r.effect_kind='create_session'").
		Where("o.order_id=? AND p.id=? AND o.collection_id=? AND o.session_id=? AND o.provider_id=? AND o.store_id=? AND o.state='paid' AND o.applied=TRUE AND o.dispatched_at IS NOT NULL", order, payment, collection, session, provider, s.StoreID).
		Clauses(clause.Locking{Strength: "SHARE"}).Take(&trace).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return trace, ErrConflict
	}
	if e != nil {
		return trace, ErrUnavailable
	}
	return trace, nil
}

func (s Service) verifySupplemental(ctx context.Context, tx *gorm.DB, r operation, req payments.SessionRequest) error {
	trace, e := s.supplementalTrace(tx, r.OrderID, r.PaymentID, r.CollectionID, r.SessionID, r.ProviderID)
	if e != nil {
		return e
	}
	if trace.ScopeID != r.ScopeID || trace.OperationID != r.CheckoutOperationID || trace.RemoteID != r.RemotePaymentID || subtle.ConstantTimeCompare(trace.RequestSHA256, r.RequestSHA256) != 1 {
		return ErrConflict
	}
	amount, e := payments.MollieSettlementAmount(req.Amount)
	if e != nil || amount != trace.Amount {
		return ErrConflict
	}

	expected, ok := new(big.Rat).SetString(req.Amount)
	if !ok {
		return ErrConflict
	}
	for _, value := range [][2]string{{trace.PaymentAmount, trace.PaymentRaw}, {trace.TransactionAmount, trace.TransactionRaw}} {
		text, e := exact(value[0], value[1])
		if e != nil {
			return ErrConflict
		}
		number, ok := new(big.Rat).SetString(text)
		if !ok || number.Cmp(expected) != 0 {
			return ErrConflict
		}
	}
	var captures []struct{ ID, Amount, RawAmount string }
	if tx.Table("payment_captures").Select("id,CAST(amount AS CHAR) AS amount,raw_amount").Where("payment_id=? AND deleted_at IS NULL", r.PaymentID).Limit(2).Clauses(clause.Locking{Strength: "SHARE"}).Find(&captures).Error != nil {
		return ErrUnavailable
	}
	if len(captures) != 1 || captures[0].ID != "cap_supp_"+trace.OperationID {
		return ErrConflict
	}
	text, e := exact(captures[0].Amount, captures[0].RawAmount)
	if e != nil {
		return ErrConflict
	}
	captured, ok := new(big.Rat).SetString(text)
	if !ok || captured.Cmp(expected) != 0 {
		return ErrConflict
	}
	created, e := payments.ReadSessionCreation(ctx, tx, key(r), s.Ring)
	if e != nil {
		return ErrUnavailable
	}
	defer clear(created.Data)
	var remote struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(created.Data, &remote) != nil || remote.ID != r.RemotePaymentID {
		return ErrConflict
	}
	stored := string(trace.Data)
	if len(stored) > 2*protected.MaxPlaintext || !strings.HasPrefix(stored, "ers1.") {
		return ErrUnavailable
	}
	encoded := strings.TrimPrefix(stored, "ers1.")
	envelope, e := base64.StdEncoding.Strict().DecodeString(encoded)
	if e != nil || base64.StdEncoding.EncodeToString(envelope) != encoded {
		return ErrUnavailable
	}
	defer clear(envelope)
	paid, e := s.Ring.Open(protected.Context{Purpose: "order-payment-v1", Table: "payments", Column: "data", Row: r.PaymentID}, envelope)
	if e != nil {
		return ErrUnavailable
	}
	defer clear(paid)
	observation := payments.SessionObservation{PaymentID: r.RemotePaymentID, ProviderStatus: "paid", Action: "successful", Amount: req.Amount, Currency: req.Currency, Session: payments.CreatedSession{Status: "authorized", Data: paid}}
	if payments.ValidateMollieObservation(req, r.RemotePaymentID, observation) != nil {
		return ErrConflict
	}
	return nil
}
