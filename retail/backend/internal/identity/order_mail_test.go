package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/eurorefurb/retail/internal/ordermail"
	"github.com/eurorefurb/retail/internal/protected"
	"gorm.io/gorm"
	"sync"
	"sync/atomic"
	"testing"
)

type orderSender struct {
	fp      string
	prepare func(maildelivery.OrderPlaced) ([]byte, error)
	send    func(context.Context, string, []byte) maildelivery.Result
}

func (s orderSender) Fingerprint() string                                      { return s.fp }
func (s orderSender) PrepareOrder(in maildelivery.OrderPlaced) ([]byte, error) { return s.prepare(in) }
func (s orderSender) Send(ctx context.Context, key string, b []byte) maildelivery.Result {
	return s.send(ctx, key, b)
}
func checkOrderMail(t *testing.T, ctx context.Context, db *gorm.DB, orderID string, ring *protected.Ring) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("order mail fixture")
		}
	}
	var other []string
	if db.Table("order_notification_outbox").Select("id").Where("order_id<>? AND status='pending'", orderID).Scan(&other).Error != nil {
		t.Fatal("mail fixture")
	}
	if len(other) > 0 {
		exec("UPDATE order_notification_outbox SET status='failed' WHERE id IN ?", other)
		defer exec("UPDATE order_notification_outbox SET status='pending' WHERE id IN ?", other)
	}
	var job string
	if db.Table("order_notification_outbox").Select("id").Where("order_id=?", orderID).Scan(&job).Error != nil || job == "" {
		t.Fatal("mail not queued")
	}
	store := ordermail.Store{DB: db, Ring: ring}
	var calls atomic.Int32
	var first []byte
	sender := orderSender{fp: string(bytes.Repeat([]byte{'a'}, 64)), prepare: func(in maildelivery.OrderPlaced) ([]byte, error) {
		if in.Amount != "158.75" || in.Currency != "eur" || in.To == "" || len(in.Reference) != 19 {
			t.Fatal("order mail facts differ")
		}
		return json.Marshal(in)
	}, send: func(_ context.Context, key string, body []byte) maildelivery.Result {
		if key != "mail/op_"+job {
			t.Error("unstable order mail key")
		}
		calls.Add(1)
		var state string
		if db.Table("order_notification_outbox").Select("status").Where("id=?", job).Scan(&state).Error != nil || state != "sending" {
			t.Error("provider called before committed claim")
		}
		if first == nil {
			first = append([]byte(nil), body...)
			return maildelivery.Result{Code: "network_error", Retry: true}
		}
		if !bytes.Equal(first, body) {
			t.Error("retry body changed")
		}
		return maildelivery.Result{ID: "00000000-0000-4000-8000-000000000001"}
	}}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := store.Dispatch(ctx, sender); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("parallel dispatch duplicated mail")
	}
	var row struct {
		Status   string
		Attempts uint
	}
	if db.Table("order_notification_outbox").Where("id=?", job).Take(&row).Error != nil || row.Status != "pending" || row.Attempts != 1 {
		t.Fatal("network retry lost")
	}
	exec("UPDATE order_mail_dispatch_gate SET next_claim_at='1970-01-01' WHERE id=1")
	exec("UPDATE order_mail_deliveries SET next_attempt_at='1970-01-01' WHERE notification_id=?", job)
	exec("UPDATE order_notification_outbox SET status='sending' WHERE id=?", job)
	exec("UPDATE order_mail_deliveries SET lease_until=UTC_TIMESTAMP(6)+INTERVAL 1 MINUTE,lease_token='00000000-0000-4000-8000-000000000003' WHERE notification_id=?", job)
	if leased, e := store.Dispatch(ctx, sender); e != nil || leased.ID != "" || calls.Load() != 1 {
		t.Fatal("live lease duplicated send")
	}
	exec("UPDATE order_mail_deliveries SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE notification_id=?", job)
	result, e := store.Dispatch(ctx, sender)
	if e != nil || result.Status != "sent" || result.Attempts != 2 || calls.Load() != 2 {
		t.Fatalf("order retry did not send: %v state=%s", e, result.Status)
	}
	var encrypted struct{ RequestEnvelope []byte }
	if db.Table("order_mail_deliveries").Select("request_envelope").Where("notification_id=?", job).Take(&encrypted).Error != nil {
		t.Fatal("mail request read")
	}
	plain, openErr := ring.Open(protected.Context{Purpose: "order-mail-request-v1", Table: "order_mail_deliveries", Column: "request_envelope", Row: job}, encrypted.RequestEnvelope)
	if openErr != nil || !bytes.Equal(plain, first) || bytes.Contains(encrypted.RequestEnvelope, first) {
		t.Fatal("request not sealed")
	}
	if _, e = store.Dispatch(ctx, sender); e != nil || calls.Load() != 2 {
		t.Fatal("sent mail dispatched again")
	}
	// Changed credentials stop a saved request rather than sending it from a new account.
	exec("UPDATE order_notification_outbox SET status='pending' WHERE id=?", job)
	exec("UPDATE order_mail_deliveries SET next_attempt_at=NULL WHERE notification_id=?", job)
	exec("UPDATE order_mail_dispatch_gate SET next_claim_at='1970-01-01' WHERE id=1")
	changed := sender
	changed.fp = string(bytes.Repeat([]byte{'b'}, 64))
	result, e = store.Dispatch(ctx, changed)
	if e != nil || result.Status != "failed" || calls.Load() != 2 {
		t.Fatal("changed sender used saved body")
	}
	// Broken row-bound ciphertext never falls back to plaintext or issues a send.
	exec("UPDATE order_notification_outbox SET status='pending' WHERE id=?", job)
	exec("UPDATE order_mail_deliveries SET request_envelope='broken',next_attempt_at=NULL WHERE notification_id=?", job)
	if _, e = store.Dispatch(ctx, sender); e != ordermail.ErrUnavailable || calls.Load() != 2 {
		t.Fatal("corrupt request sent")
	}
	// Expired ambiguous retry must stop before any network call.
	exec("UPDATE order_notification_outbox SET status='sending' WHERE id=?", job)
	exec("UPDATE order_mail_deliveries SET first_attempt_at=UTC_TIMESTAMP(6)-INTERVAL 24 HOUR,lease_until=NULL,next_attempt_at=NULL WHERE notification_id=?", job)
	exec("UPDATE order_mail_dispatch_gate SET next_claim_at='1970-01-01' WHERE id=1")
	result, e = store.Dispatch(ctx, sender)
	if e != nil || result.Status != "failed" || calls.Load() != 2 {
		t.Fatal("expired ambiguous mail resent")
	}
	t.Log(`ORDER_MAIL_REHEARSAL {"real_mysql":true,"sealed_request":true,"concurrent_single_dispatch":true,"committed_lease_before_external_send":true,"live_lease_blocks":true,"expired_lease_recovers":true,"lost_response_stable_body_and_key":true,"sent_not_resent":true,"sender_change_stopped":true,"ciphertext_corruption_stopped":true,"expired_ambiguous_retry_stopped":true,"real_external_send":false}`)
}
