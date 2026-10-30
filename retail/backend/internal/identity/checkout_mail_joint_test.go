package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/notificationmail"
	"github.com/eurorefurb/retail/internal/ordermail"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Consumes the four orders created through the original Astro checkout and
// production Gin completion. Replaces only the provider transport; business
// intents, captures, sender configuration, leases, retries and audit are real.
func checkCheckoutMailJoint(t *testing.T, ctx context.Context, db *gorm.DB, paymentRing *protected.Ring, orderIDs []string) {
	t.Helper()
	if len(orderIDs) != 4 {
		t.Fatal("four browser orders required")
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if db.WithContext(ctx).Exec(q, args...).Error != nil {
			t.Fatal("checkout mail fixture SQL failed")
		}
	}
	var unrelated []string
	if db.Table("order_notification_outbox").Select("id").Where("order_id NOT IN ? AND status='pending'", orderIDs).Scan(&unrelated).Error != nil {
		t.Fatal("mail isolation failed")
	}
	if len(unrelated) > 0 {
		exec("UPDATE order_notification_outbox SET status='failed' WHERE id IN ?", unrelated)
		defer exec("UPDATE order_notification_outbox SET status='pending' WHERE id IN ?", unrelated)
	}
	merchantRing, e := protected.New("fixture", map[string][]byte{"fixture": bytes.Repeat([]byte{94}, 32)})
	if e != nil {
		t.Fatal("mail keyring failed")
	}
	mailbox := notificationmail.Store{DB: db, Ring: merchantRing}
	if _, e := mailbox.OrderSender(ctx); e == nil {
		t.Fatal("unconfigured order sender selected")
	}
	actor := identity.User{ID: "checkout-mail-admin", Email: "checkout-mail-admin@example.invalid", Name: "Mail fixture", Role: "admin", Enabled: true, AuthVersion: 1, PasswordHash: "disabled-fixture-login", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if db.Create(&actor).Error != nil || db.Transaction(func(tx *gorm.DB) error { return authorization.Bootstrap(ctx, tx, actor.ID) }) != nil {
		t.Fatal("explicit mail authority failed")
	}
	directory, e := mailbox.Directory(ctx, actor)
	if e != nil {
		t.Fatal("mail directory failed")
	}
	if _, e = mailbox.Save(ctx, actor, notificationmail.Command{OperationID: uuid.NewString(), ExpectedRevision: directory.Revision, Name: "EuroRefurb Checkout", Email: "checkout-mail@example.invalid", ReplyTo: "reply-checkout@example.invalid", APIKey: "re_checkout_artificial_key", Enabled: true}); e != nil {
		t.Fatal("approved checkout mail configuration failed")
	}
	type facts struct {
		ID, Email, CurrencyCode, Locale, CustomDisplayID string
		DisplayID                                        uint64
	}
	var orders []facts
	if db.Table("orders").Where("id IN ?", orderIDs).Scan(&orders).Error != nil || len(orders) != 4 {
		t.Fatal("checkout mail order facts missing")
	}
	expected := map[string]facts{}
	for _, o := range orders {
		expected[o.ID] = o
	}
	var mu sync.Mutex
	bodies, providerIDs, recipients := map[string][]byte{}, map[string]string{}, map[string]bool{}
	calls, english, chinese := 0, 0, 0
	firstKey := ""
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	http.DefaultTransport = jointMailTransport(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if r.Method != "POST" || r.URL.String() != "https://api.resend.com/emails" || r.Header.Get("Authorization") != "Bearer re_checkout_artificial_key" {
			t.Error("checkout provider endpoint or credentials differ")
			return nil, errors.New("fixture")
		}
		key := r.Header.Get("Idempotency-Key")
		id := strings.TrimPrefix(key, "mail/op_")
		if !strings.HasPrefix(key, "mail/op_") {
			t.Error("checkout mail key missing")
			return nil, errors.New("fixture")
		}
		var job struct{ OrderID, Status string }
		if db.Table("order_notification_outbox").Where("id=?", id).Take(&job).Error != nil || job.Status != "sending" {
			t.Error("provider called before committed mail claim")
			return nil, errors.New("fixture")
		}
		o, ok := expected[job.OrderID]
		body, e := io.ReadAll(r.Body)
		var message struct {
			From, Subject, Text, HTML string
			ReplyTo                   string `json:"reply_to"`
			To                        []string
		}
		if !ok || e != nil || json.Unmarshal(body, &message) != nil || len(message.To) != 1 || message.To[0] != o.Email || !strings.HasSuffix(o.Email, "@example.invalid") || !strings.Contains(message.From, "checkout-mail@example.invalid") || message.ReplyTo != "reply-checkout@example.invalid" || !strings.Contains(message.Text, "158.75 EUR") || !strings.Contains(message.Text, o.CustomDisplayID) || o.CurrencyCode != "eur" {
			t.Error("checkout mail facts differ from captured order")
			return nil, errors.New("fixture")
		}
		if saved, seen := bodies[key]; seen {
			if !bytes.Equal(saved, body) {
				t.Error("lost-response mail changed body")
			}
		} else {
			bodies[key] = append([]byte(nil), body...)
			providerIDs[key] = uuid.NewString()
			recipients[o.Email] = true
			if strings.HasPrefix(strings.ToLower(o.Locale), "zh") {
				chinese++
				if !strings.Contains(message.Subject, "订单确认") {
					t.Error("Chinese checkout mail lost locale")
				}
			} else {
				english++
				if !strings.Contains(message.Subject, "Order confirmation") {
					t.Error("English checkout mail lost locale")
				}
			}
		}
		if firstKey == "" {
			firstKey = key
		}
		// The fake provider has already accepted this key and body. Lose its
		// acknowledgement three times, then return the same provider identity.
		if key == firstKey && calls <= 3 {
			return nil, errors.New("artificial response loss")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"` + providerIDs[key] + `"}`))}, nil
	})
	sender, e := mailbox.OrderSender(ctx)
	if e != nil {
		t.Fatal("approved checkout sender unavailable")
	}
	store := ordermail.Store{DB: db, Ring: paymentRing}
	resetGate := func() { exec("UPDATE order_mail_dispatch_gate SET next_claim_at='1970-01-01' WHERE id=1") }
	resetGate()
	// SQL dispatch pacing admits one external call even across concurrent workers.
	var group sync.WaitGroup
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, e := store.Dispatch(ctx, sender); e != nil {
				t.Error("concurrent mail worker failed")
			}
		}()
	}
	group.Wait()
	if calls != 1 || firstKey == "" {
		t.Fatal("concurrent checkout mail duplicated provider dispatch")
	}
	firstID := strings.TrimPrefix(firstKey, "mail/op_")
	// Keep only this job eligible while accelerating its isolated retry clock.
	var siblings []string
	if db.Table("order_notification_outbox").Select("id").Where("order_id IN ? AND id<>?", orderIDs, firstID).Scan(&siblings).Error != nil || len(siblings) != 3 {
		t.Fatal("checkout sibling jobs missing")
	}
	exec("UPDATE order_notification_outbox SET status='failed' WHERE id IN ?", siblings)
	for attempt := 2; attempt <= 3; attempt++ {
		resetGate()
		exec("UPDATE order_mail_deliveries SET next_attempt_at='1970-01-01' WHERE notification_id=?", firstID)
		out, e := store.Dispatch(ctx, sender)
		wanted := "pending"
		if attempt == 3 {
			wanted = "failed"
		}
		if e != nil || out.ID != firstID || out.Status != wanted || out.Attempts != uint(attempt) {
			t.Fatal("ambiguous checkout mail retry state differs")
		}
	}
	retry := notificationmail.RetryOrderJob{OperationID: uuid.NewString(), ExpectedAttempts: 3}
	if out, e := mailbox.RetryOrderJob(ctx, actor, firstID, retry); e != nil || out.Replayed {
		t.Fatal("checkout mail recovery failed")
	}
	resetGate()
	out, e := store.Dispatch(ctx, sender)
	if e != nil || out.ID != firstID || out.Status != "sent" || out.Attempts != 1 || calls != 4 {
		t.Fatal("recovered checkout mail not accepted")
	}
	if replay, e := mailbox.RetryOrderJob(ctx, actor, firstID, retry); e != nil || !replay.Replayed {
		t.Fatal("sent recovery receipt replay failed")
	}
	var status string
	if db.Table("order_notification_outbox").Select("status").Where("id=?", firstID).Scan(&status).Error != nil || status != "sent" {
		t.Fatal("recovery replay requeued accepted mail")
	}
	exec("UPDATE order_notification_outbox SET status='pending' WHERE id IN ?", siblings)
	for i := 0; i < 3; i++ {
		resetGate()
		out, e := store.Dispatch(ctx, sender)
		if e != nil || out.Status != "sent" {
			t.Fatal("sibling checkout mail failed")
		}
	}
	resetGate()
	if empty, e := store.Dispatch(ctx, sender); e != nil || empty.ID != "" {
		t.Fatal("accepted checkout mail dispatched twice")
	}
	if calls != 7 || len(bodies) != 4 || len(providerIDs) != 4 || len(recipients) != 4 || english != 2 || chinese != 2 {
		t.Fatal("checkout mail recipient, locale or deduplication count differs")
	}
	var n int64
	if db.Table("order_notification_outbox").Where("order_id IN ? AND status='sent'", orderIDs).Count(&n).Error != nil || n != 4 {
		t.Fatal("checkout mail durable sent count differs")
	}
	if db.Table("audit_events").Where("action='order_mail.retry' AND resource_id=?", firstID).Count(&n).Error != nil || n != 1 {
		t.Fatal("checkout mail recovery audit differs")
	}
	for key, body := range bodies {
		id := strings.TrimPrefix(key, "mail/op_")
		var saved struct {
			RequestEnvelope []byte
			ProviderID      string
		}
		if db.Table("order_mail_deliveries").Where("notification_id=?", id).Take(&saved).Error != nil || saved.ProviderID != providerIDs[key] || bytes.Contains(saved.RequestEnvelope, body) {
			t.Fatal("checkout mail encrypted request or provider identity differs")
		}
		plain, e := paymentRing.Open(protected.Context{Purpose: "order-mail-request-v1", Table: "order_mail_deliveries", Column: "request_envelope", Row: id}, saved.RequestEnvelope)
		if e != nil || !bytes.Equal(plain, body) {
			t.Fatal("checkout frozen request differs")
		}
		clear(plain)
	}
	t.Log(`CHECKOUT_MAIL_JOINT_SQL {"browser_paid_orders":4,"production_resend_adapter":true,"approved_database_sender":true,"captured_total":"158.75 EUR","english_messages":2,"chinese_messages":2,"concurrent_single_dispatch":true,"ambiguous_attempts":3,"manual_recovery_with_one_audit":true,"stable_body_key_provider_identity":true,"provider_requests":7,"unique_provider_messages":4,"sent_not_requeued_on_receipt_replay":true,"encrypted_frozen_requests":true,"real_external_send":false}`)
}
