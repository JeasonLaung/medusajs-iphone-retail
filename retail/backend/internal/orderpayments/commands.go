// Package orderpayments stages native payments for an already completed order.
// These internal commands grant no customer session and perform no provider call.
package orderpayments

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math/big"
	"net/url"
	"regexp"
	"time"

	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInput        = errors.New("invalid order payment command")
	ErrDenied       = errors.New("order payment unavailable for this account")
	ErrConflict     = errors.New("order payment command conflict")
	ErrUnavailable  = errors.New("order payment service unavailable")
	idPattern       = regexp.MustCompile(`^[A-Za-z0-9_-]{1,191}$`)
	currencyPattern = regexp.MustCompile(`^[a-z]{3}$`)
	amountPattern   = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})\.[0-9]{2}$`)
)

const provider = "pp_mollie-hosted-checkout_mollie"

// Principal is trusted server input. A customer session coordinator must resolve
// and revalidate its token inside this transaction; never decode this from JSON.
type Principal struct {
	AccountID   string
	AuthVersion uint64
}
type Input struct {
	OperationID         string `json:"operation_id"`
	PreviousOperationID string `json:"previous_operation_id,omitempty"`
	OrderID             string `json:"order_id"`
	ExpectedVersion     int    `json:"expected_version"`
	CollectionID        string `json:"collection_id"`
	Amount              string `json:"amount"`
	CurrencyCode        string `json:"currency_code"`
}
type Result struct {
	OperationID         string  `json:"operation_id"`
	PreviousOperationID *string `json:"previous_operation_id,omitempty"`
	OrderID             string  `json:"order_id"`
	OrderVersion        int     `json:"order_version"`
	CollectionID        string  `json:"collection_id"`
	Amount              string  `json:"amount"`
	CurrencyCode        string  `json:"currency_code"`
	State               string  `json:"state"`
	Applied             bool    `json:"applied"`
	CanCancel           bool    `json:"can_cancel"`
	CanRetry            bool    `json:"can_retry"`
}
type Service struct {
	DB                    *gorm.DB
	StoreID, ReturnOrigin string
	Ring                  *protected.Ring
	Adapters              *payments.CreateAdapters
}
type operation struct {
	OperationID                                                       string
	PreviousOperationID                                               *string
	AccountID                                                         string
	AuthVersion                                                       uint64
	StoreID, OrderID                                                  string
	OrderVersion                                                      int
	CartID, CollectionID, SessionID, Amount, CurrencyCode, ProviderID string
	InputSHA256, RequestSHA256                                        []byte
	State                                                             string
	DispatchedAt                                                      *time.Time
	Applied                                                           bool
}

func (operation) TableName() string { return "order_payment_operations" }
func result(r operation) Result {
	return Result{r.OperationID, r.PreviousOperationID, r.OrderID, r.OrderVersion, r.CollectionID, r.Amount, r.CurrencyCode, r.State, r.Applied, r.State == "pending" && r.DispatchedAt == nil, terminal(r.State)}
}
func terminal(s string) bool { return s == "failed" || s == "canceled" || s == "expired" }
func validUUID(s string) bool {
	id, e := uuid.Parse(s)
	return e == nil && id.Version() == 4 && id.String() == s
}
func valid(in Input) bool {
	if !validUUID(in.OperationID) || in.PreviousOperationID != "" && (!validUUID(in.PreviousOperationID) || in.PreviousOperationID == in.OperationID) || !idPattern.MatchString(in.OrderID) || !idPattern.MatchString(in.CollectionID) || in.ExpectedVersion < 1 || !amountPattern.MatchString(in.Amount) || !currencyPattern.MatchString(in.CurrencyCode) {
		return false
	}
	n, ok := new(big.Rat).SetString(in.Amount)
	return ok && n.Sign() > 0
}

func txReady(tx *gorm.DB) bool {
	if tx == nil || tx.Statement == nil {
		return false
	}
	_, ok := tx.Statement.ConnPool.(gorm.TxCommitter)
	return ok
}
func runtime(tx *gorm.DB, enabled bool) error {
	var row struct {
		Version int
		Enabled bool
	}
	if tx.Table("order_payment_runtime").Where("id=1").Clauses(clause.Locking{Strength: "SHARE"}).Take(&row).Error != nil || row.Version != 1 || enabled && !row.Enabled {
		return ErrUnavailable
	}
	return nil
}
func authority(tx *gorm.DB, p Principal) error {
	if !idPattern.MatchString(p.AccountID) || len(p.AccountID) > 80 || p.AuthVersion == 0 {
		return ErrDenied
	}
	var a struct {
		AuthVersion uint64
		Enabled     bool
		VerifiedAt  *time.Time
	}
	e := tx.Table("customer_accounts").Select("auth_version,enabled,verified_at").Where("id=?", p.AccountID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&a).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return ErrDenied
	}
	if e != nil {
		return ErrUnavailable
	}
	if !a.Enabled || a.VerifiedAt == nil || a.AuthVersion != p.AuthVersion {
		return ErrDenied
	}
	return nil
}

// Immutable completed checkout trace and explicit new-account ownership only.
// Email, orders.customer_id and historic customer subjects grant no authority.
// The original cart is evidence, not a current bearer capability: rotation,
// revocation or soft deletion never revokes the attached account ownership.
func (s Service) scope(tx *gorm.DB, p Principal, order string) (string, error) {
	if !idPattern.MatchString(s.StoreID) {
		return "", ErrUnavailable
	}
	var owner struct{ CartID, StoreID string }
	e := tx.Table("customer_order_ownership").Select("cart_id,store_id").Where("order_id=? AND customer_id=?", order, p.AccountID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&owner).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return "", ErrDenied
	}
	if e != nil {
		return "", ErrUnavailable
	}
	if owner.StoreID != s.StoreID {
		return "", ErrDenied
	}
	var rows []struct{ CartID string }
	e = tx.Table("cart_completion_receipts r").Select("r.cart_id").Joins("JOIN cart_ownership o ON o.cart_id=r.cart_id").Joins("JOIN cart_checkout_attempts a ON a.cart_id=r.cart_id AND a.operation_id=r.operation_id AND a.generation=r.generation").Joins("JOIN order_carts l ON l.order_id=r.order_id AND l.cart_id=r.cart_id").Joins("JOIN carts c ON c.id=r.cart_id AND c.completed_at IS NOT NULL").Where("r.order_id=? AND r.cart_id=? AND o.store_id=?", order, owner.CartID, s.StoreID).Limit(2).Clauses(clause.Locking{Strength: "SHARE"}).Find(&rows).Error
	if e != nil {
		return "", ErrUnavailable
	}
	if len(rows) != 1 {
		return "", ErrDenied
	}
	return rows[0].CartID, nil
}
func lockOrder(tx *gorm.DB, id string) error {
	var row struct{ ID string }
	if tx.Table("orders").Select("id").Where("id=? AND deleted_at IS NULL", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
		return ErrConflict
	}
	return nil
}
func key(r operation) payments.EffectKey {
	k := payments.EffectKey{ScopeID: r.OrderID, OperationID: r.OperationID, Kind: "create_session", ResourceID: r.SessionID}
	copy(k.Digest[:], r.RequestSHA256)
	return k
}
func (s Service) request(ctx context.Context, tx *gorm.DB, r operation) (payments.SessionRequest, error) {
	if len(r.RequestSHA256) != 32 || s.Ring == nil || !amountPattern.MatchString(r.Amount) {
		return payments.SessionRequest{}, ErrUnavailable
	}
	req, e := payments.ReadSessionRequest(ctx, tx, key(r), s.Ring)
	if e != nil || req.ProviderID != provider || req.ProviderID != r.ProviderID || req.CollectionID != r.CollectionID || req.SessionID != r.SessionID || req.Currency != r.CurrencyCode {
		return payments.SessionRequest{}, ErrUnavailable
	}
	n, ok := new(big.Rat).SetString(req.Amount)
	want, ok2 := new(big.Rat).SetString(r.Amount)
	if !ok || !ok2 || n.Cmp(want) != 0 {
		return payments.SessionRequest{}, ErrUnavailable
	}
	return req, nil
}
func inputHash(store string, p Principal, in Input) [32]byte {
	b, _ := json.Marshal(struct {
		Store     string
		Principal Principal
		Input     Input
	}{store, p, in})
	return sha256.Sum256(b)
}
func (s Service) Prepare(ctx context.Context, p Principal, in Input) (Result, error) {
	var out Result
	if s.DB == nil {
		return out, ErrUnavailable
	}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var e error; out, e = s.PrepareTx(ctx, tx, p, in); return e })
	return out, e
}

// PrepareTx stages one protected request and pending local session atomically.
// Retrying this UUID authenticates its original immutable input; it never sends.
func (s Service) PrepareTx(ctx context.Context, tx *gorm.DB, p Principal, in Input) (Result, error) {
	fail := func(e error) (Result, error) { return Result{}, e }
	if !valid(in) {
		return fail(ErrInput)
	}
	if !txReady(tx) || s.Ring == nil || s.Adapters == nil {
		return fail(ErrUnavailable)
	}
	u, e := url.Parse(s.ReturnOrigin)
	if e != nil || u == nil || u.String() != s.ReturnOrigin || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.Scheme != "https" && (u.Scheme != "http" || u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") {
		return fail(ErrUnavailable)
	}
	if _, e = s.Adapters.Lookup(provider); e != nil {
		return fail(ErrUnavailable)
	}
	tx = tx.WithContext(ctx)
	if e = authority(tx, p); e != nil {
		return fail(e)
	}
	if e = runtime(tx, true); e != nil {
		return fail(e)
	}
	cart, e := s.scope(tx, p, in.OrderID)
	if e != nil {
		return fail(e)
	}
	if e = lockOrder(tx, in.OrderID); e != nil {
		return fail(e)
	}
	hash := inputHash(s.StoreID, p, in)
	var prior operation
	e = tx.Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&prior).Error
	if e == nil {
		if prior.AccountID != p.AccountID || prior.AuthVersion != p.AuthVersion || prior.StoreID != s.StoreID || prior.OrderID != in.OrderID || prior.CartID != cart || subtle.ConstantTimeCompare(prior.InputSHA256, hash[:]) != 1 {
			return fail(ErrConflict)
		}
		if _, e = s.request(ctx, tx, prior); e != nil {
			return fail(e)
		}
		return result(prior), nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return fail(ErrUnavailable)
	}
	bound, e := orders.ReadSupplementalPaymentTx(ctx, tx, in.OrderID, in.ExpectedVersion)
	if e != nil {
		if errors.Is(e, orders.ErrVersionConflict) {
			return fail(ErrConflict)
		}
		return fail(ErrUnavailable)
	}
	if bound.CollectionID != in.CollectionID || bound.Amount != in.Amount || bound.CurrencyCode != in.CurrencyCode {
		return fail(ErrConflict)
	}
	if e = s.fresh(tx, p, in, cart); e != nil {
		return fail(e)
	}
	row := operation{OperationID: in.OperationID, AccountID: p.AccountID, AuthVersion: p.AuthVersion, StoreID: s.StoreID, OrderID: in.OrderID, OrderVersion: bound.Version, CartID: cart, CollectionID: bound.CollectionID, SessionID: "payses_" + uuid.NewString(), Amount: bound.Amount, CurrencyCode: bound.CurrencyCode, ProviderID: provider, InputSHA256: hash[:], State: "pending"}
	if in.PreviousOperationID != "" {
		prev := in.PreviousOperationID
		row.PreviousOperationID = &prev
	}
	redirect := s.ReturnOrigin + "/api/mollie-return?" + url.Values{"cart_id": {cart}, "order_id": {row.OrderID}, "order_payment_id": {row.OperationID}}.Encode()
	data, _ := json.Marshal(map[string]string{"cart_id": cart, "session_id": row.SessionID, "redirect_url": redirect})
	contextData, _ := json.Marshal(map[string]string{"idempotency_key": row.SessionID})
	req := payments.SessionRequest{ProviderID: provider, CollectionID: row.CollectionID, SessionID: row.SessionID, Amount: row.Amount, Currency: row.CurrencyCode, Data: data, Context: contextData}
	k, _, e := payments.StageSessionRequest(ctx, tx, key(row), s.Ring, req)
	if e != nil {
		return fail(e)
	}
	row.RequestSHA256 = k.Digest[:]
	encrypted, encryptedContext, e := payments.SealSessionFields(s.Ring, req)
	if e != nil {
		return fail(e)
	}
	raw, _ := json.Marshal(map[string]any{"value": row.Amount, "precision": 100})
	now := gorm.Expr("UTC_TIMESTAMP(6)")
	if tx.Table("payment_sessions").Create(map[string]any{"id": row.SessionID, "payment_collection_id": row.CollectionID, "provider_id": provider, "currency_code": row.CurrencyCode, "amount": row.Amount, "raw_amount": string(raw), "data": encrypted, "context": encryptedContext, "status": "pending", "created_at": now, "updated_at": now}).Error != nil {
		return fail(ErrUnavailable)
	}
	if tx.Table("order_payment_operations").Create(map[string]any{"operation_id": row.OperationID, "previous_operation_id": row.PreviousOperationID, "account_id": row.AccountID, "auth_version": row.AuthVersion, "store_id": row.StoreID, "order_id": row.OrderID, "order_version": row.OrderVersion, "cart_id": row.CartID, "collection_id": row.CollectionID, "session_id": row.SessionID, "amount": row.Amount, "currency_code": row.CurrencyCode, "provider_id": provider, "input_sha256": hash[:], "request_sha256": row.RequestSHA256, "state": "pending", "created_at": now, "updated_at": now}).Error != nil {
		return fail(ErrUnavailable)
	}
	return result(row), nil
}

// Called only after account, scope and order locks and current balance validation.
func (s Service) fresh(tx *gorm.DB, p Principal, in Input, cart string) error {
	var holds []struct{ OperationID string }
	if tx.Table("order_payment_operations").Select("operation_id").Where("active_order=? OR active_collection=?", in.OrderID, in.CollectionID).Limit(1).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&holds).Error != nil {
		return ErrUnavailable
	}
	if len(holds) != 0 {
		return ErrConflict
	}
	var refunds []struct{ OperationID string }
	if tx.Table("order_refund_operations").Select("operation_id").Where("order_id=? AND canceled_at IS NULL AND applied=FALSE AND state NOT IN('failed','canceled')", in.OrderID).Limit(1).Clauses(clause.Locking{Strength: "SHARE"}).Find(&refunds).Error != nil {
		return ErrUnavailable
	}
	if len(refunds) != 0 {
		return ErrConflict
	}
	var history []operation
	if tx.Where("collection_id=?", in.CollectionID).Order("created_at DESC,operation_id DESC").Limit(101).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&history).Error != nil || len(history) > 100 {
		return ErrUnavailable
	}
	if len(history) == 0 && in.PreviousOperationID != "" || len(history) != 0 && in.PreviousOperationID == "" {
		return ErrConflict
	}
	var previous *operation
	for i := range history {
		h := &history[i]
		if h.OrderID != in.OrderID || h.StoreID != s.StoreID || h.CartID != cart || h.AccountID != p.AccountID || !terminal(h.State) || h.PreviousOperationID != nil && *h.PreviousOperationID == in.PreviousOperationID {
			return ErrConflict
		}
		if h.OperationID == in.PreviousOperationID {
			previous = h
		}
	}
	if len(history) != 0 && previous == nil {
		return ErrConflict
	}
	// Even soft-deleted old payments may have external effects. Never borrow a
	// cart collection or bypass an unknown session by changing its local status.
	for _, table := range []string{"payments", "cart_payment_collections"} {
		var rows []struct{ ID string }
		if tx.Table(table).Select("id").Where("payment_collection_id=?", in.CollectionID).Limit(1).Clauses(clause.Locking{Strength: "SHARE"}).Find(&rows).Error != nil {
			return ErrUnavailable
		}
		if len(rows) != 0 {
			return ErrConflict
		}
	}
	var sessions []struct{ ID, Status string }
	if tx.Table("payment_sessions").Select("id,status").Where("payment_collection_id=?", in.CollectionID).Limit(101).Clauses(clause.Locking{Strength: "SHARE"}).Find(&sessions).Error != nil || len(sessions) > 100 {
		return ErrUnavailable
	}
	for _, v := range sessions {
		known := false
		for _, h := range history {
			if h.SessionID == v.ID && terminal(h.State) && (v.Status == "canceled" || v.Status == "error") {
				known = true
				break
			}
		}
		if !known {
			return ErrConflict
		}
	}
	var original struct{ ID, RegionID, ProviderID string }
	if tx.Table("cart_completion_receipts r").Select("p.id,o.region_id,s.provider_id").Joins("JOIN orders o ON o.id=r.order_id AND o.deleted_at IS NULL").Joins("JOIN cart_checkout_attempts a ON a.cart_id=r.cart_id AND a.operation_id=r.operation_id").Joins("JOIN payments p ON p.id=r.payment_id AND p.payment_collection_id=a.collection_id AND p.payment_session_id=a.session_id AND p.captured_at IS NOT NULL AND p.deleted_at IS NULL").Joins("JOIN payment_sessions s ON s.id=p.payment_session_id AND s.status='captured' AND s.deleted_at IS NULL").Where("r.order_id=? AND r.cart_id=?", in.OrderID, cart).Clauses(clause.Locking{Strength: "SHARE"}).Take(&original).Error != nil || original.ProviderID != provider {
		return ErrConflict
	}
	var allowed []struct{ ID string }
	if tx.Table("payment_providers p").Select("p.id").Joins("JOIN commerce_region_payment_providers r ON r.payment_provider_id=p.id AND r.deleted_at IS NULL").Where("p.id=? AND p.is_enabled=TRUE AND p.deleted_at IS NULL AND r.region_id=?", provider, original.RegionID).Limit(2).Clauses(clause.Locking{Strength: "SHARE"}).Find(&allowed).Error != nil {
		return ErrUnavailable
	}
	if len(allowed) != 1 {
		return ErrConflict
	}
	return nil
}
