package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkMollieNotifications(t *testing.T, ctx context.Context, db *gorm.DB) {
	t.Helper()
	exec := func(sql string, args ...any) {
		t.Helper()
		if db.WithContext(ctx).Exec(sql, args...).Error != nil {
			t.Fatal("notification fixture SQL failed")
		}
	}
	tx := func(fn func(*gorm.DB) error) error { return db.WithContext(ctx).Transaction(fn) }
	ring, _ := protected.New("fixture", map[string][]byte{"fixture": bytes.Repeat([]byte{0x39}, 32)})
	provider := "pp_mollie-hosted-checkout_mollie"
	remote := "tr_notification_owned"
	request := payments.SessionRequest{ProviderID: provider, CollectionID: "notification-collection", SessionID: "payses_notification_owned", Amount: "1.005", Currency: "eur", Data: json.RawMessage(`{"cart_id":"cart_notification","redirect_url":"https://shop.example/api/mollie-return?cart_id=cart_notification","session_id":"payses_notification_owned"}`)}
	key := payments.EffectKey{ScopeID: "notification-cart", OperationID: uuid.NewString(), Kind: "create_session", ResourceID: request.SessionID}
	var receipt payments.EffectReceipt
	if err := tx(func(tx *gorm.DB) error {
		var err error
		key, _, err = payments.StageSessionRequest(ctx, tx, key, ring, request)
		if err != nil {
			return err
		}
		receipt, _, err = payments.ClaimSessionRequest(ctx, tx, key, ring, 30*time.Second)
		return err
	}); err != nil || !receipt.Dispatch {
		t.Fatal("notification request fixture failed")
	}
	response := payments.CreatedSession{Status: "pending", Data: json.RawMessage(`{"id":"tr_notification_owned","status":"open","amount":{"value":"1.01","currency":"EUR"},"metadata":{"session_id":"payses_notification_owned","cart_id":"cart_notification"},"large":9007199254740993.000000000000000000001}`)}
	exec("CREATE TRIGGER mollie_reference_fault BEFORE INSERT ON mollie_payment_references FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial reference failure'")
	if err := tx(func(tx *gorm.DB) error {
		_, err := payments.RecordSessionCreation(ctx, tx, key, receipt.AttemptID, ring, response)
		return err
	}); err != payments.ErrEffectStorage {
		t.Fatal("reference fault did not roll back creation")
	}
	exec("DROP TRIGGER mollie_reference_fault")
	var original struct {
		State          string
		ResultEnvelope []byte
	}
	if db.Table("payment_effect_journal").Where("scope_id=?", key.ScopeID).Take(&original).Error != nil || original.State != "inflight" || original.ResultEnvelope != nil {
		t.Fatal("reference failure left partial creation result")
	}

	if err := tx(func(tx *gorm.DB) error {
		_, err := payments.RecordSessionCreation(ctx, tx, key, receipt.AttemptID, ring, response)
		return err
	}); err != nil {
		t.Fatal("creation did not register durable remote reference")
	}
	enqueue := func(id string) (bool, error) {
		var known bool
		err := tx(func(tx *gorm.DB) error {
			var err error
			known, err = payments.EnqueueMollieNotification(ctx, tx, provider, id)
			return err
		})
		return known, err
	}
	if _, err := enqueue(remote); err != payments.ErrSchema {
		t.Fatal("notification gate default open")
	}
	exec("UPDATE mollie_notification_runtime SET enabled=TRUE WHERE id=1")
	if known, err := enqueue("tr_unknown"); err != nil || known {
		t.Fatal("unknown notification created reference")
	}
	var unknownRows int64
	if db.Table("mollie_notification_inbox").Where("remote_id='tr_unknown'").Count(&unknownRows).Error != nil || unknownRows != 0 {
		t.Fatal("unknown notification grew storage")
	}
	h, err := payments.NewMollieWebhook(db, payments.MollieWebhookOptions{ProviderIDs: []string{provider}})
	if err != nil {
		t.Fatal("notification HTTP fixture failed")
	}
	router := gin.New()
	h.Register(router)
	post := func(body, contentType string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/hooks/payment/mollie-hosted-checkout_mollie", strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	unknown := post("id=tr_unknown", "application/x-www-form-urlencoded")
	known := post("id="+remote, "application/x-www-form-urlencoded")
	if known.Code != 200 || unknown.Code != 200 || known.Body.String() != unknown.Body.String() || known.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("notification existence leaked or ACK failed")
	}
	for _, body := range []string{"id=" + remote + "&id=tr_other", "id=" + remote + "&status=paid", "id=../../other", "", "id=" + strings.Repeat("x", 2048)} {
		if post(body, "application/x-www-form-urlencoded").Code != 400 {
			t.Fatal("invalid webhook body accepted")
		}
	}
	if post(`{"id":"tr_notification_owned"}`, "application/json").Code != 415 {
		t.Fatal("unexpected notification encoding accepted")
	}
	read := func() payments.MollieNotification {
		t.Helper()
		var row payments.MollieNotification
		if db.Table("mollie_notification_inbox").Where("provider_id=? AND remote_id=?", provider, remote).Take(&row).Error != nil {
			t.Fatal("notification row unavailable")
		}
		return row
	}
	claim := func() (payments.MollieNotification, error) {
		var row payments.MollieNotification
		err := tx(func(tx *gorm.DB) error {
			var err error
			row, err = payments.ClaimMollieNotification(ctx, tx, provider, remote, ring, 30*time.Second)
			return err
		})
		return row, err
	}
	var wg sync.WaitGroup
	var winners atomic.Int32
	var winner payments.MollieNotification
	var mu sync.Mutex
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row, err := claim()
			if err != nil {
				t.Error("notification claim race failed")
				return
			}
			if row.Dispatch {
				winners.Add(1)
				mu.Lock()
				winner = row
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 || winner.ClaimedGeneration != 1 || winner.Request.SessionID != request.SessionID {
		t.Fatal("notification concurrent claim not single")
	}
	observed := payments.SessionObservation{PaymentID: remote, ProviderStatus: "paid", Action: "successful", Amount: "1.01", Currency: "eur", Session: payments.CreatedSession{Status: "authorized", Data: json.RawMessage(`{"id":"tr_notification_owned","status":"paid","amount":{"value":"1.01","currency":"EUR"},"metadata":{"session_id":"payses_notification_owned","cart_id":"cart_notification"}}`)}}
	complete := func(c payments.MollieNotification, o payments.SessionObservation) error {
		return tx(func(tx *gorm.DB) error { return payments.CompleteMollieNotification(ctx, tx, c, o, ring) })
	}
	if known, err := enqueue(remote); err != nil || !known {
		t.Fatal("notification during in-flight claim failed")
	}
	bad := observed
	bad.Amount = "1.014"
	if err := complete(winner, bad); err != payments.ErrEffectConflict {
		t.Fatal("notification amount was rounded into a false match")
	}
	rollback := errors.New("artificial notification rollback")
	if err := tx(func(tx *gorm.DB) error {
		if err := payments.CompleteMollieNotification(ctx, tx, winner, observed, ring); err != nil {
			return err
		}
		return rollback
	}); err != rollback || read().ProcessedGeneration != 0 {
		t.Fatal("rolled-back observation ACK persisted")
	}
	exec("CREATE TRIGGER mollie_evidence_fault BEFORE INSERT ON mollie_notification_observations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial evidence failure'")
	if err := complete(winner, observed); err != payments.ErrEffectStorage || read().ProcessedGeneration != 0 {
		t.Fatal("evidence failure ACKed notification")
	}
	exec("DROP TRIGGER mollie_evidence_fault")

	if err := complete(winner, observed); err != nil {
		t.Fatal("verified notification could not complete")
	}
	if row := read(); row.Generation != 2 || row.ProcessedGeneration != 1 || row.State != "pending" {
		t.Fatal("newer notification was lost by old ACK")
	}
	if err := complete(winner, observed); err != nil {
		t.Fatal("notification completion replay failed")
	}
	changed := observed
	changed.Action = "failed"
	if err := complete(winner, changed); err != payments.ErrEffectConflict {
		t.Fatal("notification replay changed saved evidence")
	}
	next, err := claim()
	if err != nil || !next.Dispatch || next.ClaimedGeneration != 2 {
		t.Fatal("new generation not claimable")
	}
	exec("UPDATE mollie_notification_inbox SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE remote_id=?", remote)
	reclaimed, err := claim()
	if err != nil || !reclaimed.Dispatch || *reclaimed.AttemptID == *next.AttemptID {
		t.Fatal("expired notification GET not recoverable")
	}
	if err := complete(next, observed); err != payments.ErrEffectConflict {
		t.Fatal("old notification attempt ACKed new lease")
	}
	if err := tx(func(tx *gorm.DB) error { return payments.RetryMollieNotification(ctx, tx, reclaimed, time.Minute) }); err != nil {
		t.Fatal("notification retry failed")
	}
	if row, err := claim(); err != nil || row.Dispatch {
		t.Fatal("notification backoff ignored")
	}
	exec("UPDATE mollie_notification_inbox SET available_at=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE remote_id=?", remote)
	var calls atomic.Int32
	adapter := executionObserver{create: func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		t.Fatal("notification worker attempted create")
		return payments.CreateUncertain, payments.CreatedSession{}, nil
	}, observe: func(c context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
		calls.Add(1)
		if id != remote || r.SessionID != request.SessionID {
			return payments.SessionObservation{}, errors.New("incorrect notification request")
		}
		if err := db.WithContext(c).Transaction(func(tx *gorm.DB) error {
			known, err := payments.EnqueueMollieNotification(c, tx, provider, remote)
			if !known && err == nil {
				return errors.New("missing remote reference")
			}
			return err
		}); err != nil {
			return payments.SessionObservation{}, err
		}
		return observed, nil
	}}
	registry, _ := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{provider: adapter})
	if result, err := payments.ProcessMollieNotification(ctx, db, provider, remote, ring, registry); err != nil || !result.Called || calls.Load() != 1 {
		t.Fatal("notification observer worker failed")
	}
	if row := read(); row.State != "pending" || row.ProcessedGeneration != 2 || row.Generation != 3 {
		t.Fatal("worker swallowed notification arriving during GET")
	}
	adapter.observe = func(context.Context, payments.SessionRequest, string) (payments.SessionObservation, error) {
		calls.Add(1)
		return observed, nil
	}
	registry, _ = payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{provider: adapter})
	if result, err := payments.ProcessMollieNotification(ctx, db, provider, remote, ring, registry); err != nil || !result.Called || read().State != "idle" {
		t.Fatal("notification generation not completed")
	}
	if result, err := payments.ProcessMollieNotification(ctx, db, provider, remote, ring, registry); err != nil || result.Called || calls.Load() != 2 {
		t.Fatal("idle notification repeated GET")
	}
	var evidence struct {
		ResultEnvelope []byte
		ConsumedAt     *time.Time
	}
	if db.Table("mollie_notification_observations").Where("attempt_id=?", *winner.AttemptID).Take(&evidence).Error != nil || bytes.Contains(evidence.ResultEnvelope, []byte("tr_notification_owned")) || evidence.ConsumedAt != nil {
		t.Fatal("notification evidence plaintext or falsely business-consumed")
	}
	if err := tx(func(tx *gorm.DB) error {
		saved, err := payments.ReadMollieNotificationObservation(ctx, tx, *winner.AttemptID, ring)
		if err != nil {
			return err
		}
		if saved.ProviderStatus != "paid" || saved.Action != "successful" || saved.Amount != "1.01" {
			t.Fatal("evidence did not authenticate facts")
		}
		return nil
	}); err != nil {
		t.Fatal("encrypted evidence unreadable")
	}
	savedEnvelope := append([]byte(nil), evidence.ResultEnvelope...)
	evidence.ResultEnvelope[len(evidence.ResultEnvelope)-1] ^= 1
	exec("UPDATE mollie_notification_observations SET result_envelope=? WHERE attempt_id=?", evidence.ResultEnvelope, *winner.AttemptID)
	if err := tx(func(tx *gorm.DB) error {
		_, err := payments.ReadMollieNotificationObservation(ctx, tx, *winner.AttemptID, ring)
		return err
	}); err != payments.ErrEffectProtection {
		t.Fatal("corrupt observation returned facts")
	}
	exec("UPDATE mollie_notification_observations SET result_envelope=? WHERE attempt_id=?", savedEnvelope, *winner.AttemptID)
	var due []payments.MollieNotificationLocator
	if err := tx(func(tx *gorm.DB) error {
		var err error
		due, err = payments.DueMollieNotifications(ctx, tx, 100)
		return err
	}); err != nil || len(due) != 0 {
		t.Fatal("idle notification listed as due")
	}

	exec("CREATE TRIGGER notification_enqueue_fault BEFORE UPDATE ON mollie_notification_inbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial enqueue fault'")
	if post("id="+remote, "application/x-www-form-urlencoded").Code != 503 {
		t.Fatal("notification ACKed failed commit")
	}
	exec("DROP TRIGGER notification_enqueue_fault")
	_, _ = enqueue(remote)
	adapter.observe = func(context.Context, payments.SessionRequest, string) (payments.SessionObservation, error) {
		return payments.SessionObservation{}, errors.New("private-provider-error")
	}
	registry, _ = payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{provider: adapter})
	if result, err := payments.ProcessMollieNotification(ctx, db, provider, remote, ring, registry); err != payments.ErrCreateExecution || !result.Called || read().State != "pending" {
		t.Fatal("failed GET was ACKed or not retried")
	}
	// Multiple untrusted notifications coalesce without losing their generations.
	beforeGeneration := read().Generation
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if known, err := enqueue(remote); err != nil || !known {
				t.Error("concurrent enqueue failed")
			}
		}()
	}
	wg.Wait()
	if read().Generation != beforeGeneration+8 {
		t.Fatal("concurrent notification increments lost")
	}
	exec("UPDATE mollie_notification_inbox SET available_at=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE remote_id=?", remote)
	if err := tx(func(tx *gorm.DB) error {
		var err error
		due, err = payments.DueMollieNotifications(ctx, tx, 1)
		return err
	}); err != nil || len(due) != 1 || due[0].RemoteID != remote {
		t.Fatal("durable pending notification undiscoverable")
	}
	disabled, _ := payments.NewMollieWebhook(nil, payments.MollieWebhookOptions{})
	disabledRouter := gin.New()
	disabled.Register(disabledRouter)
	disabledResponse := httptest.NewRecorder()
	disabledRouter.ServeHTTP(disabledResponse, httptest.NewRequest("POST", "/hooks/payment/mollie-hosted-checkout_mollie", strings.NewReader("id="+remote)))
	if disabledResponse.Code != 503 {
		t.Fatal("unconfigured callback open")
	}

	exec("UPDATE mollie_notification_runtime SET enabled=FALSE WHERE id=1")
	if post("id="+remote, "application/x-www-form-urlencoded").Code != 503 {
		t.Fatal("disabled notification route ACKed")
	}
	if _, err := payments.EnqueueMollieNotification(ctx, db, provider, remote); err != payments.ErrEffectInput {
		t.Fatal("notification write accepted autocommit")
	}
	t.Log(`MOLLIE_NOTIFICATIONS_REHEARSAL {"migrations":82,"reference_atomic_with_creation":true,"reference_failure_rolls_back_creation":true,"evidence_failure_no_ack":true,"corrupt_evidence_denied":true,"due_scan_bounded":true,"concurrent_enqueue_generations":true,"unknown_id_no_storage":true,"http_known_unknown_same_ack":true,"strict_form_body":true,"concurrent_claim_single":true,"new_generation_not_lost":true,"observation_encrypted_unconsumed":true,"rollback_no_ack":true,"stale_attempt_fenced":true,"expired_get_reclaim":true,"backoff_respected":true,"worker_get_outside_sql":true,"idle_no_get":true,"provider_error_retry":true,"failed_commit_no_ack":true,"default_disabled":true,"real_provider_called":false,"order_payment_updated":false}`)
}
