package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
)

var ErrRefundExecution = errors.New("refund requires reconciliation")
var mollieRefundID = regexp.MustCompile(`^re_[A-Za-z0-9_]{1,120}$`)
var refundDecimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,35})(\.[0-9]{1,2})?$`)

// RefundRequest is private server input, not a browser DTO or authorization.
// The coordinator must first commit a unique immutable operation, authorize the
// current order/return/payment, reserve the exact remaining captured balance and
// claim dispatch. Calls run outside SQL. A transport error never permits a new
// POST; recovery is ObserveRefund/DiscoverRefund using THIS original request.
type RefundRequest struct {
	Session         SessionRequest `json:"session"`
	RemotePaymentID string         `json:"remote_payment_id"`
	OperationID     string         `json:"operation_id"`
	OrderID         string         `json:"order_id"`
	RequestID       string         `json:"request_id"`
	Amount          string         `json:"amount"`
}

// RefundObservation is a verified provider fact, not proof of a local refund
// movement. In particular queued/pending/processing must NOT be booked as paid
// back. Raw responses, metadata, actor identifiers and keys are never returned.
type RefundObservation struct {
	RefundID  string    `json:"refund_id"`
	PaymentID string    `json:"payment_id"`
	Status    string    `json:"status"`
	Amount    string    `json:"amount"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
}

type RefundAdapter interface {
	CreateRefund(context.Context, RefundRequest) (RefundObservation, error)
	ObserveRefund(context.Context, RefundRequest, string) (RefundObservation, error)
	DiscoverRefund(context.Context, RefundRequest) (RefundObservation, error)
}

// Refunder reuses the explicitly configured provider and merchant credential;
// there is no fallback provider or separate browser-selected refund transport.
func (a *CreateAdapters) Refunder(providerID string) (RefundAdapter, error) {
	adapter, err := a.Lookup(providerID)
	if err != nil {
		return nil, err
	}
	refunder, ok := adapter.(RefundAdapter)
	if !ok || refunder == nil {
		return nil, ErrEffectInput
	}
	return refunder, nil
}

func refundBoundary(out *RefundObservation, err *error) {
	if recover() != nil || *err != nil {
		*out = RefundObservation{}
		*err = ErrRefundExecution
	}
}
func (a *MollieCreateAdapter) refundInput(in RefundRequest) (string, error) {
	if a == nil || a.transport == nil || a.transport.client == nil || !molliePaymentID.MatchString(in.RemotePaymentID) || !effectID.MatchString(in.OrderID) || !effectID.MatchString(in.RequestID) || !refundDecimal.MatchString(in.Amount) {
		return "", ErrRefundExecution
	}
	id, e := uuid.Parse(in.OperationID)
	if e != nil || id.Version() != 4 || id.String() != in.OperationID {
		return "", ErrRefundExecution
	}
	body, _, _, e := a.createBody(in.Session)
	clear(body)
	if e != nil {
		return "", ErrRefundExecution
	}
	amount, e := mollieAmount(in.Amount)
	original, e2 := mollieAmount(in.Session.Amount)
	n, ok := new(big.Rat).SetString(amount)
	total, ok2 := new(big.Rat).SetString(original)
	if e != nil || e2 != nil || !ok || !ok2 || n.Sign() <= 0 || n.Cmp(total) > 0 {
		return "", ErrRefundExecution
	}
	return amount, nil
}
func refundMetadata(in RefundRequest) map[string]string {
	return map[string]string{"refund_operation_id": in.OperationID, "order_id": in.OrderID, "after_sales_request_id": in.RequestID}
}
func (a *MollieCreateAdapter) refundJSON(ctx context.Context, method, path string, body []byte, key string, status int) (json.RawMessage, error) {
	req, e := http.NewRequestWithContext(ctx, method, "https://api.mollie.com/v2/payments/"+path, bytes.NewReader(body))
	if e != nil {
		return nil, ErrRefundExecution
	}
	req.Header.Set("Authorization", "Bearer "+a.transport.apiKey)
	req.Header.Set("Accept", "application/hal+json")
	if method == http.MethodPost {
		// A bytes.Reader installs GetBody. Together with Idempotency-Key it
		// permits net/http to replay POST after a reused connection breaks.
		// Only the durable coordinator may decide recovery, never the transport.
		req.GetBody = nil
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
	}
	response, e := a.transport.client.Do(req)
	if e != nil {
		return nil, ErrRefundExecution
	}
	defer response.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	defer clear(raw)
	if e != nil || response.StatusCode != status || len(raw) > 1<<20 {
		return nil, ErrRefundExecution
	}
	parsed, e := requestObject(raw, false)
	if e != nil {
		return nil, ErrRefundExecution
	}
	return parsed, nil
}
func (a *MollieCreateAdapter) verifyRefund(raw json.RawMessage, in RefundRequest, expected string) (RefundObservation, error) {
	var v struct {
		Resource, ID, Mode, Status string
		PaymentID                  string `json:"paymentId"`
		Amount                     struct{ Value, Currency string }
		Metadata                   struct {
			OperationID string `json:"refund_operation_id"`
			OrderID     string `json:"order_id"`
			RequestID   string `json:"after_sales_request_id"`
		}
		CreatedAt string `json:"createdAt"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return RefundObservation{}, ErrRefundExecution
	}
	amount, e := totals.Resolve(totals.Number{Decimal: v.Amount.Value})
	want, e2 := totals.Resolve(totals.Number{Decimal: in.Amount})
	created, e3 := time.Parse(time.RFC3339Nano, v.CreatedAt)
	if e != nil || e2 != nil || e3 != nil || created.IsZero() || v.Resource != "refund" || !mollieRefundID.MatchString(v.ID) || expected != "" && v.ID != expected || v.PaymentID != in.RemotePaymentID || v.Mode != strings.SplitN(a.transport.apiKey, "_", 2)[0] || v.Amount.Currency != strings.ToUpper(in.Session.Currency) || amount != want || v.Metadata.OperationID != in.OperationID || v.Metadata.OrderID != in.OrderID || v.Metadata.RequestID != in.RequestID {
		return RefundObservation{}, ErrRefundExecution
	}
	switch v.Status {
	case "queued", "pending", "processing", "refunded", "failed", "canceled":
	default:
		return RefundObservation{}, ErrRefundExecution
	}
	return RefundObservation{v.ID, v.PaymentID, v.Status, amount, in.Session.Currency, created}, nil
}

// CreateRefund sends one POST with the original stable operation ID. It first
// verifies the parent paid payment with the configured merchant's GET and the
// original saved session/cart/mode/amount/currency, then its remaining balance.
// No redirects, retries, body-selected URLs or local SQL writes are performed.
func (a *MollieCreateAdapter) CreateRefund(ctx context.Context, in RefundRequest) (out RefundObservation, err error) {
	defer refundBoundary(&out, &err)
	amount, e := a.refundInput(in)
	if e != nil {
		return out, ErrRefundExecution
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	parent, e := a.ObserveSession(ctx, in.Session, in.RemotePaymentID)
	if e != nil || parent.ProviderStatus != "paid" {
		return out, ErrRefundExecution
	}
	defer clear(parent.Session.Data)
	var balance struct {
		Remaining *struct{ Value, Currency string } `json:"amountRemaining"`
	}
	if json.Unmarshal(parent.Session.Data, &balance) != nil || balance.Remaining == nil || balance.Remaining.Currency != strings.ToUpper(in.Session.Currency) {
		return out, ErrRefundExecution
	}
	remaining, e := totals.Resolve(totals.Number{Decimal: balance.Remaining.Value})
	n, ok := new(big.Rat).SetString(remaining)
	requested, _ := new(big.Rat).SetString(amount)
	total, _ := new(big.Rat).SetString(parent.Amount)
	if e != nil || !ok || n.Cmp(requested) < 0 || n.Cmp(total) > 0 {
		return out, ErrRefundExecution
	}
	body, _ := json.Marshal(map[string]any{"amount": map[string]string{"value": amount, "currency": strings.ToUpper(in.Session.Currency)}, "description": "after-sales:" + in.RequestID, "metadata": refundMetadata(in)})
	defer clear(body)
	raw, e := a.refundJSON(ctx, http.MethodPost, in.RemotePaymentID+"/refunds", body, in.OperationID, http.StatusCreated)
	if e != nil {
		return out, ErrRefundExecution
	}
	defer clear(raw)
	return a.verifyRefund(raw, in, "")
}

// ObserveRefund is safe after a full refund: remaining balance may be zero.
// The original payment is still independently verified; only the exact original
// operation metadata and parent refund can satisfy this read. 404 is uncertain.
func (a *MollieCreateAdapter) ObserveRefund(ctx context.Context, in RefundRequest, id string) (out RefundObservation, err error) {
	defer refundBoundary(&out, &err)
	if _, e := a.refundInput(in); e != nil || !mollieRefundID.MatchString(id) {
		return out, ErrRefundExecution
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	parent, e := a.ObserveSession(ctx, in.Session, in.RemotePaymentID)
	if e != nil || parent.ProviderStatus != "paid" {
		return out, ErrRefundExecution
	}
	clear(parent.Session.Data)
	raw, e := a.refundJSON(ctx, http.MethodGet, in.RemotePaymentID+"/refunds/"+id, nil, "", http.StatusOK)
	if e != nil {
		return out, ErrRefundExecution
	}
	defer clear(raw)
	return a.verifyRefund(raw, in, id)
}
