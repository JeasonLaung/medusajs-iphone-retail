package orderpayments

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/url"
	"strings"

	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type View struct {
	Result
	CheckoutURL *string `json:"checkout_url"`
}
type Consent struct {
	ExpectedVersion     int    `json:"expected_version"`
	CollectionID        string `json:"collection_id"`
	Amount              string `json:"amount"`
	CurrencyCode        string `json:"currency_code"`
	PreviousOperationID string `json:"previous_operation_id,omitempty"`
}
type Context struct {
	OrderID           string   `json:"order_id"`
	OrderVersion      int      `json:"order_version"`
	CurrencyCode      string   `json:"currency_code"`
	PendingDifference string   `json:"pending_difference"`
	Next              *Consent `json:"next"`
	Payments          []View   `json:"payments"`
}

// A URL is a private read of authenticated creation evidence, never payment
// evidence or a provider call. Current balance/version must still match.
func (s Service) view(ctx context.Context, tx *gorm.DB, row operation) (View, error) {
	out := View{Result: result(row)}
	if row.DispatchedAt == nil || row.Applied || terminal(row.State) || row.State == "authorized" || row.State == "paid" {
		return out, nil
	}
	bound, e := orders.ReadSupplementalPaymentTx(ctx, tx, row.OrderID, row.OrderVersion)
	if errors.Is(e, orders.ErrVersionConflict) {
		return out, nil
	}
	if e != nil {
		return View{}, ErrUnavailable
	}
	if bound.CollectionID != row.CollectionID || bound.Amount != row.Amount || bound.CurrencyCode != row.CurrencyCode {
		return out, nil
	}
	receipt, e := payments.InspectEffect(ctx, tx, key(row))
	if e != nil {
		return View{}, ErrUnavailable
	}
	if receipt.State != "succeeded" {
		return out, nil
	}
	req, e := s.request(ctx, tx, row)
	if e != nil {
		return View{}, e
	}
	defer clear(req.Data)
	defer clear(req.Context)
	created, e := payments.ReadSessionCreation(ctx, tx, key(row), s.Ring)
	if e != nil {
		return View{}, ErrUnavailable
	}
	defer clear(created.Data)
	var remote struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Links  struct {
			Checkout struct {
				Href string `json:"href"`
			} `json:"checkout"`
		} `json:"_links"`
	}
	if json.Unmarshal(created.Data, &remote) != nil {
		return View{}, ErrUnavailable
	}
	if remote.Status != "open" && remote.Status != "pending" {
		return out, nil
	}
	action := "pending"
	if remote.Status == "open" {
		action = "not_supported"
	}
	if payments.ValidateMollieObservation(req, remote.ID, payments.SessionObservation{PaymentID: remote.ID, ProviderStatus: remote.Status, Action: action, Amount: req.Amount, Currency: req.Currency, Session: created}) != nil {
		return View{}, ErrUnavailable
	}
	link, e := checkoutLink(remote.Links.Checkout.Href)
	if e != nil {
		return View{}, e
	}
	out.CheckoutURL = &link
	return out, nil
}
func checkoutLink(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || len(raw) > 2048 || u == nil || u.String() != raw || u.Scheme != "https" || u.User != nil || u.Opaque != "" || u.Fragment != "" || u.Hostname() != "mollie.com" && !strings.HasSuffix(u.Hostname(), ".mollie.com") || u.Port() != "" && u.Port() != "443" {
		return "", ErrUnavailable
	}
	return u.String(), nil
}
func (s Service) ViewTx(ctx context.Context, tx *gorm.DB, p Principal, order, id string) (View, error) {
	if !idPattern.MatchString(order) || !validUUID(id) {
		return View{}, ErrInput
	}
	cart, e := s.private(ctx, tx, p, order)
	if e != nil {
		return View{}, e
	}
	row, e := s.ownedOperation(ctx, tx, p, order, cart, id)
	if e != nil {
		return View{}, e
	}
	return s.view(ctx, tx, row)
}

// ContextTx uses locking current reads even when the authenticated account
// coordinator already established a repeatable-read snapshot before the order lock.
func (s Service) ContextTx(ctx context.Context, tx *gorm.DB, p Principal, order string) (Context, error) {
	out := Context{OrderID: order, Payments: []View{}}
	if !idPattern.MatchString(order) {
		return Context{}, ErrInput
	}
	cart, e := s.private(ctx, tx, p, order)
	if e != nil {
		return Context{}, e
	}
	var current struct {
		Version      int
		CurrencyCode string
	}
	if tx.Table("orders").Select("version,currency_code").Where("id=?", order).Clauses(clause.Locking{Strength: "SHARE"}).Take(&current).Error != nil {
		return Context{}, ErrUnavailable
	}
	out.OrderVersion = current.Version
	out.CurrencyCode = current.CurrencyCode
	balance, e := orders.CurrentPendingDifferenceTx(tx, order)
	if e != nil {
		return Context{}, ErrUnavailable
	}
	text := strings.TrimRight(strings.TrimRight(balance.FloatString(128), "0"), ".")
	exact, ok := new(big.Rat).SetString(text)
	if !ok || exact.Cmp(balance) != 0 {
		return Context{}, ErrUnavailable
	}
	out.PendingDifference = text
	var rows []operation
	if tx.Where("order_id=?", order).Order("created_at,operation_id").Limit(1001).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&rows).Error != nil || len(rows) > 1000 {
		return Context{}, ErrUnavailable
	}
	for _, row := range rows {
		if row.AccountID != p.AccountID || row.StoreID != s.StoreID || row.CartID != cart {
			return Context{}, ErrDenied
		}
		if _, e := s.request(ctx, tx, row); e != nil {
			return Context{}, e
		}
		view, e := s.view(ctx, tx, row)
		if e != nil {
			return Context{}, e
		}
		out.Payments = append(out.Payments, view)
	}
	var gate struct{ Enabled bool }
	if tx.Table("order_payment_runtime").Select("enabled").Where("id=1").Clauses(clause.Locking{Strength: "SHARE"}).Take(&gate).Error != nil {
		return Context{}, ErrUnavailable
	}
	if !gate.Enabled {
		return out, nil
	}
	bound, e := orders.ReadSupplementalPaymentTx(ctx, tx, order, current.Version)
	if errors.Is(e, orders.ErrVersionConflict) {
		return out, nil
	}
	if e != nil {
		return Context{}, ErrUnavailable
	}
	previous := ""
	for _, r := range rows {
		if r.CollectionID != bound.CollectionID {
			continue
		}
		successor := false
		for _, other := range rows {
			if other.PreviousOperationID != nil && *other.PreviousOperationID == r.OperationID {
				successor = true
				break
			}
		}
		if !successor {
			if previous != "" {
				return Context{}, ErrConflict
			}
			previous = r.OperationID
		}
	}
	in := Input{PreviousOperationID: previous, OrderID: order, ExpectedVersion: bound.Version, CollectionID: bound.CollectionID, Amount: bound.Amount, CurrencyCode: bound.CurrencyCode}
	if e = s.fresh(tx, p, in, cart); e != nil {
		if errors.Is(e, ErrConflict) {
			return out, nil
		}
		return Context{}, e
	}
	out.Next = &Consent{ExpectedVersion: bound.Version, CollectionID: bound.CollectionID, Amount: bound.Amount, CurrencyCode: bound.CurrencyCode, PreviousOperationID: previous}
	return out, nil
}
