package cart

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const hostedMollie = "pp_mollie-hosted-checkout_mollie"

// Internal settings. Caller supplies registered adapters and independent keys;
// neither a provider selection nor redirect origin comes from browser JSON.
type InitialPaymentOptions struct {
	Ring            *protected.Ring
	Adapters        *payments.CreateAdapters
	ReturnOrigin    string
	ManualProviders []string
}
type InitialPaymentCommand struct {
	OperationID string
	Version     string
}
type initialAttempt struct {
	Activated                                                           bool
	ObservedStatus                                                      *string
	NeedsReview                                                         bool
	Generation                                                          string
	InputVersion, CartVersion                                           uint64
	CollectionID, SessionID, CartAmount, SettlementAmount, CurrencyCode string
	RequestSHA256                                                       []byte
}
type InitialPaymentResult struct {
	CartID      string  `json:"cart_id"`
	Version     string  `json:"version"`
	OperationID string  `json:"operation_id"`
	State       string  `json:"state"`
	CheckoutURL *string `json:"checkout_url"`
}

func initialOptions(o InitialPaymentOptions) error {
	u, err := url.Parse(o.ReturnOrigin)
	if err != nil || u == nil || u.String() != o.ReturnOrigin || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.Scheme != "https" && (u.Scheme != "http" || u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || o.Ring == nil || o.Adapters == nil {
		return ErrConfiguration
	}
	if _, err = o.Adapters.Lookup(hostedMollie); err != nil {
		return ErrConfiguration
	}
	return nil
}
func initialReady(ctx context.Context, tx *gorm.DB) error {
	if err := commandReady(ctx, tx, "cart.access"); err != nil {
		return err
	}
	if payments.Ready(ctx, tx) != nil {
		return ErrSchema
	}
	var gate struct {
		Version int
		Enabled bool
	}
	r := tx.Raw("SELECT version,enabled FROM cart_checkout_runtime WHERE id=1 FOR SHARE").Scan(&gate)
	if r.Error != nil || r.RowsAffected != 1 || gate.Version != 1 || !gate.Enabled {
		return ErrSchema
	}
	return nil
}
func initialProvider(tx *gorm.DB, current Snapshot) error {
	var ids []string
	r := tx.Table("payment_providers p").Select("p.id").Joins("JOIN commerce_region_payment_providers r ON r.payment_provider_id=p.id AND r.deleted_at IS NULL").Where("p.id=? AND p.is_enabled=TRUE AND p.deleted_at IS NULL AND r.region_id=?", hostedMollie, current.RegionID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(2).Scan(&ids)
	if r.Error != nil {
		return ErrStorage
	}
	if len(ids) != 1 {
		return ErrConfiguration
	}
	return nil
}
func initialWhere(tx *gorm.DB, ref sessionstore.CartReference, in InitialPaymentCommand) *gorm.DB {
	return tx.Table("cart_checkout_attempts").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID)
}
func initialKey(ref sessionstore.CartReference, in InitialPaymentCommand, row initialAttempt) payments.EffectKey {
	key := payments.EffectKey{ScopeID: ref.CartID, OperationID: in.OperationID, Kind: "create_session", ResourceID: row.SessionID}
	copy(key.Digest[:], row.RequestSHA256)
	return key
}

// stageInitialPayment runs in the caller's transaction. Refresh precedes freeze;
// the version, collection, encrypted request and immutable binding commit as one.
func stageInitialPayment(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in InitialPaymentCommand, o InitialPaymentOptions) (initialAttempt, payments.EffectKey, error) {
	fail := func(err error) (initialAttempt, payments.EffectKey, error) {
		return initialAttempt{}, payments.EffectKey{}, err
	}
	if _, err := validateShipping(ShippingCommand{OperationID: in.OperationID, Version: in.Version, OptionID: "checkout"}); err != nil {
		return fail(err)
	}
	if !refOK(store, ref) {
		return fail(ErrInput)
	}
	if err := initialReady(ctx, tx); err != nil {
		return fail(err)
	}
	channel, err := storeChannel(ctx, tx, store)
	if err != nil {
		return fail(err)
	}
	current, err := lockCurrentAuthority(ctx, tx, store, channel, ref, "UPDATE", "", in.OperationID)
	if err != nil {
		return fail(err)
	}
	if err := initialProvider(tx, current); err != nil {
		return fail(err)
	}
	var rows []initialAttempt
	if initialWhere(tx, ref, in).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1).Scan(&rows).Error != nil {
		return fail(ErrStorage)
	}
	if len(rows) == 1 {
		row := rows[0]
		v, _ := strconv.ParseUint(in.Version, 10, 64)
		if row.InputVersion != v || row.CartVersion != v+1 || current.Version != strconv.FormatUint(row.CartVersion, 10) || subtle.ConstantTimeCompare([]byte(row.Generation), []byte(ref.Generation)) != 1 || len(row.RequestSHA256) != 32 {
			return fail(ErrConflict)
		}
		var freeze struct{ CheckoutAttemptID *string }
		if tx.Table("cart_ownership").Select("checkout_attempt_id").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&freeze).Error != nil || freeze.CheckoutAttemptID == nil || *freeze.CheckoutAttemptID != in.OperationID {
			return fail(ErrConflict)
		}
		amounts, err := assembleAmounts(tx, current)
		if err != nil || amounts.Totals["total"] != row.CartAmount || amounts.CurrencyCode != row.CurrencyCode {
			return fail(ErrConflict)
		}
		key := initialKey(ref, in, row)
		request, err := payments.ReadSessionRequest(ctx, tx, key, o.Ring)
		if err != nil {
			return fail(err)
		}
		if request.SessionID != row.SessionID || request.CollectionID != row.CollectionID || request.ProviderID != hostedMollie || request.Amount != row.SettlementAmount || request.Currency != row.CurrencyCode {
			return fail(ErrConflict)
		}
		var owners []string
		if tx.Table("cart_payment_collections").Select("cart_id").Where("payment_collection_id=? AND deleted_at IS NULL", row.CollectionID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(2).Scan(&owners).Error != nil || len(owners) != 1 || owners[0] != ref.CartID {
			return fail(ErrConflict)
		}
		var sessions []string
		if tx.Table("payment_sessions").Select("id").Where("payment_collection_id=? AND deleted_at IS NULL", row.CollectionID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(2).Scan(&sessions).Error != nil || len(sessions) != 1 || sessions[0] != row.SessionID {
			return fail(ErrConflict)
		}
		var binding struct{ CollectionID, SessionID, ProviderID, CurrencyCode, Amount, RawAmount, SessionCurrency, SessionAmount, SessionRaw string }
		r := tx.Table("cart_payment_collections l").Select("c.id AS collection_id,s.id AS session_id,s.provider_id,c.currency_code,CAST(c.amount AS CHAR) AS amount,c.raw_amount,s.currency_code AS session_currency,CAST(s.amount AS CHAR) AS session_amount,s.raw_amount AS session_raw").Joins("JOIN payment_collections c ON c.id=l.payment_collection_id AND c.deleted_at IS NULL").Joins("JOIN payment_sessions s ON s.payment_collection_id=c.id AND s.id=? AND s.deleted_at IS NULL", row.SessionID).Where("l.cart_id=? AND l.payment_collection_id=? AND l.deleted_at IS NULL", ref.CartID, row.CollectionID).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&binding)
		if r.Error != nil || r.RowsAffected != 1 || binding.ProviderID != hostedMollie || binding.CurrencyCode != row.CurrencyCode || binding.SessionCurrency != row.CurrencyCode {
			return fail(ErrConflict)
		}
		for _, pair := range [][2]string{{binding.Amount, binding.RawAmount}, {binding.SessionAmount, binding.SessionRaw}} {
			n, err := nonNegativeAmount(pair[0], &pair[1])
			if err != nil {
				return fail(err)
			}
			value, err := totals.Resolve(n)
			if err != nil || value != row.SettlementAmount {
				return fail(ErrConflict)
			}
		}
		return row, key, nil
	}
	if current.Version != in.Version {
		return fail(ErrConflict)
	}
	if err := shippingContactReady(tx, current); err != nil {
		return fail(err)
	}
	var links []string
	if tx.Table("cart_payment_collections").Select("payment_collection_id").Where("cart_id=? AND deleted_at IS NULL", current.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1).Scan(&links).Error != nil {
		return fail(ErrStorage)
	}
	if len(links) != 0 {
		return fail(ErrConflict)
	}
	if err := supportedSavedShipping(tx, current.ID, o.ManualProviders); err != nil {
		return fail(err)
	}
	at := time.Now().UTC()
	if _, err := RefreshItemSnapshots(ctx, tx, store, ref, in.Version); err != nil {
		return fail(err)
	}
	if _, err := RefreshItemPrices(ctx, tx, store, ref, in.Version, at); err != nil {
		return fail(err)
	}
	if _, err := RefreshManualShippingMethods(ctx, tx, store, ref, in.Version, o.ManualProviders, at); err != nil {
		return fail(err)
	}
	var selected []string
	if tx.Table("cart_shipping_methods").Select("shipping_option_id").Where("cart_id=? AND deleted_at IS NULL AND shipping_option_id IS NOT NULL", current.ID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(101).Scan(&selected).Error != nil || len(selected) > 100 {
		return fail(ErrSchema)
	}
	selection, err := ReadSelectedManualShipping(ctx, tx, store, ref, in.Version, selected, o.ManualProviders, at)
	if err != nil {
		return fail(err)
	}
	if !selection.Complete {
		return fail(ErrItemUnavailable)
	}
	if _, err := RefreshSystemTaxes(ctx, tx, store, ref, in.Version, true); err != nil {
		return fail(err)
	}
	if _, err := RefreshPromotions(ctx, tx, store, ref, in.Version, PromotionRefresh{At: at}); err != nil {
		return fail(err)
	}
	amounts, err := assembleAmounts(tx, current)
	if err != nil {
		return fail(err)
	}
	if len(amounts.Lines) == 0 {
		return fail(ErrInput)
	}
	settled, err := payments.MollieSettlementAmount(amounts.Totals["total"])
	if err != nil {
		return fail(ErrConfiguration)
	}
	settled, err = totals.Resolve(totals.Number{Decimal: settled})
	if err != nil || settled == "0" {
		return fail(ErrConfiguration)
	}
	version, _ := strconv.ParseUint(in.Version, 10, 64)
	row := initialAttempt{Generation: ref.Generation, InputVersion: version, CartVersion: version + 1, CollectionID: "paycol_" + uuid.NewString(), SessionID: "payses_" + uuid.NewString(), CartAmount: amounts.Totals["total"], SettlementAmount: settled, CurrencyCode: current.CurrencyCode}
	redirect := o.ReturnOrigin + "/api/mollie-return?cart_id=" + url.QueryEscape(current.ID)
	data, _ := json.Marshal(map[string]string{"cart_id": current.ID, "session_id": row.SessionID, "redirect_url": redirect})
	context, _ := json.Marshal(map[string]string{"idempotency_key": row.SessionID})
	request := payments.SessionRequest{ProviderID: hostedMollie, CollectionID: row.CollectionID, SessionID: row.SessionID, Amount: settled, Currency: current.CurrencyCode, Data: data, Context: context}
	key, _, err := payments.StageSessionRequest(ctx, tx, payments.EffectKey{ScopeID: current.ID, OperationID: in.OperationID, Kind: "create_session", ResourceID: row.SessionID}, o.Ring, request)
	if err != nil {
		return fail(err)
	}
	row.RequestSHA256 = key.Digest[:]
	protectedData, protectedContext, err := payments.SealSessionFields(o.Ring, request)
	if err != nil {
		return fail(err)
	}
	scalar, raw, err := sqlAmount(settled)
	if err != nil {
		return fail(err)
	}
	now := gorm.Expr("UTC_TIMESTAMP(6)")
	if tx.Table("payment_collections").Create(map[string]any{"id": row.CollectionID, "currency_code": row.CurrencyCode, "amount": scalar, "raw_amount": raw, "status": "not_paid", "created_at": now, "updated_at": now}).Error != nil {
		return fail(ErrStorage)
	}
	if tx.Table("payment_sessions").Create(map[string]any{"id": row.SessionID, "payment_collection_id": row.CollectionID, "provider_id": hostedMollie, "currency_code": row.CurrencyCode, "amount": scalar, "raw_amount": raw, "data": protectedData, "context": protectedContext, "status": "pending", "created_at": now, "updated_at": now}).Error != nil {
		return fail(ErrStorage)
	}
	if tx.Table("cart_payment_collections").Create(map[string]any{"cart_id": current.ID, "payment_collection_id": row.CollectionID, "id": "capc_" + uuid.NewString(), "created_at": now, "updated_at": now}).Error != nil {
		return fail(ErrStorage)
	}
	if tx.Table("cart_checkout_attempts").Create(map[string]any{"cart_id": current.ID, "operation_id": in.OperationID, "generation": row.Generation, "input_version": row.InputVersion, "cart_version": row.CartVersion, "collection_id": row.CollectionID, "session_id": row.SessionID, "cart_amount": row.CartAmount, "settlement_amount": row.SettlementAmount, "currency_code": row.CurrencyCode, "request_sha256": row.RequestSHA256, "created_at": now}).Error != nil {
		return fail(ErrStorage)
	}
	r := tx.Table("cart_ownership").Where("cart_id=? AND version=? AND checkout_attempt_id IS NULL", current.ID, version).Updates(map[string]any{"version": row.CartVersion, "checkout_attempt_id": in.OperationID, "updated_at": now})
	if r.Error != nil || r.RowsAffected != 1 {
		return fail(ErrConflict)
	}
	return row, key, nil
}

// StartInitialPayment owns short SQL transactions and calls the registered
// provider only after a durable claim commits. Unknown outcomes never re-POST.
// No stock reservation, payment confirmation or order is implied by this result.
func StartInitialPayment(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in InitialPaymentCommand, o InitialPaymentOptions) (InitialPaymentResult, error) {
	if db == nil || db.Statement == nil {
		return InitialPaymentResult{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return InitialPaymentResult{}, ErrInput
	}
	if err := initialOptions(o); err != nil {
		return InitialPaymentResult{}, err
	}
	var row initialAttempt
	var key payments.EffectKey
	var request payments.SessionRequest
	var receipt payments.EffectReceipt
	var adapter payments.CreateSessionAdapter
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		row, key, err = stageInitialPayment(ctx, tx, store, ref, in, o)
		if err != nil {
			return err
		}
		receipt, request, err = payments.ClaimSessionRequest(ctx, tx, key, o.Ring, 30*time.Second)
		if err != nil {
			return err
		}
		if receipt.Dispatch {
			adapter, err = o.Adapters.Lookup(request.ProviderID)
		}
		return err
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return InitialPaymentResult{}, err
	}
	if receipt.Dispatch {
		outcome := payments.CreateFailed
		var created payments.CreatedSession
		if ctx.Err() == nil {
			callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			outcome, created, _ = payments.InvokeCreate(callCtx, adapter, request)
			cancel()
		}
		savedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		err = db.WithContext(savedCtx).Transaction(func(tx *gorm.DB) error {
			var err error
			if outcome == payments.CreateSucceeded {
				receipt, err = payments.RecordSessionCreation(savedCtx, tx, key, receipt.AttemptID, o.Ring, created)
			} else {
				receipt, err = payments.RecordEffect(savedCtx, tx, key, receipt.AttemptID, string(outcome))
			}
			return err
		})
		cancel()
		if err != nil {
			return InitialPaymentResult{}, err
		}
	}
	out := InitialPaymentResult{CartID: ref.CartID, Version: strconv.FormatUint(row.CartVersion, 10), OperationID: in.OperationID, State: "processing"}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Reauthorize even after a definitive external response. The encrypted fact
		// was persisted above even if the caller lost authority or canceled.
		var err error
		row, key, err = stageInitialPayment(ctx, tx, store, ref, in, o)
		if err != nil {
			return err
		}
		receipt, err = payments.InspectEffect(ctx, tx, key)
		if err != nil {
			return err
		}
		if receipt.State == "uncertain" || receipt.State == "failed" {
			out.State = "requires_review"
			return nil
		}
		if receipt.State != "succeeded" {
			return nil
		}
		result, err := payments.ReadSessionCreation(ctx, tx, key, o.Ring)
		if err != nil {
			return err
		}
		if result.Status != "pending" && result.Status != "requires_more" {
			out.State = "requires_review"
			return nil
		}
		var properties struct {
			Links struct {
				Checkout struct {
					Href string `json:"href"`
				} `json:"checkout"`
			} `json:"_links"`
		}
		if json.Unmarshal(result.Data, &properties) != nil {
			return ErrStorage
		}
		u, err := url.Parse(properties.Links.Checkout.Href)
		if err != nil || u == nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" || u.Hostname() != "mollie.com" && !strings.HasSuffix(u.Hostname(), ".mollie.com") || u.Port() != "" && u.Port() != "443" {
			return ErrStorage
		}
		request, err := payments.ReadSessionRequest(ctx, tx, key, o.Ring)
		if err != nil {
			return err
		}
		var local struct{ Status string }
		if tx.Table("payment_sessions").Select("status").Where("id=? AND payment_collection_id=? AND deleted_at IS NULL", row.SessionID, row.CollectionID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&local).Error != nil {
			return ErrStorage
		}
		if local.Status != "pending" && local.Status != "requires_more" {
			out.State = "requires_review"
			return nil
		}
		if !row.Activated {
			request.Data = result.Data
			data, context, err := payments.SealSessionFields(o.Ring, request)
			if err != nil {
				return err
			}
			r := tx.Table("payment_sessions").Where("id=? AND payment_collection_id=? AND deleted_at IS NULL", row.SessionID, row.CollectionID).Updates(map[string]any{"data": data, "context": context, "status": result.Status, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
			if r.Error != nil || r.RowsAffected != 1 {
				return ErrStorage
			}
			if r := initialWhere(tx, ref, in).Where("activated=FALSE").Update("activated", true); r.Error != nil || r.RowsAffected != 1 {
				return ErrStorage
			}
		}
		out.State = "ready"
		value := u.String()
		out.CheckoutURL = &value
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, err
}
