package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/eurorefurb/retail/internal/protected"
	"gorm.io/gorm"
)

type deliveryOracle struct {
	sender *maildelivery.Resend
	send   func(context.Context, string, []byte) maildelivery.Result
}

func (o *deliveryOracle) Fingerprint() string { return o.sender.Fingerprint() }
func (o *deliveryOracle) Prepare(to, link, locale string) ([]byte, error) {
	return o.sender.Prepare(to, link, locale)
}
func (o *deliveryOracle) Send(ctx context.Context, key string, payload []byte) maildelivery.Result {
	return o.send(ctx, key, payload)
}

func checkCustomerDelivery(t *testing.T, ctx context.Context, db *gorm.DB, store *customer.Store, ring *protected.Ring) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if db.WithContext(ctx).Exec(query, args...).Error != nil {
			t.Fatal("verification delivery fixture SQL failed")
		}
	}
	sequence := 0
	fixture := func() (string, string, string) {
		t.Helper()
		exec("UPDATE customer_verification_mail SET status='cancelled' WHERE status='pending'")
		exec("UPDATE customer_mail_dispatch_gate SET next_claim_at='2000-01-01' WHERE id=1")
		sequence++
		account, err := store.Register(ctx, customer.Registration{Email: fmt.Sprintf("delivery-%d@example.invalid", sequence), Password: "fixture", FirstName: "New", LastName: "Customer", Country: "dk", Locale: "en"})
		if err != nil {
			t.Fatal("delivery customer fixture failed")
		}
		var row struct {
			ID      string
			Payload []byte
		}
		if db.Raw("SELECT id,payload FROM customer_verification_mail WHERE customer_id=?", account.ID).Scan(&row).Error != nil {
			t.Fatal("delivery intent missing")
		}
		plain, err := ring.Open(protected.Context{Purpose: "customer-verification", Table: "customer_verification_mail", Column: "payload", Row: row.ID}, row.Payload)
		if err != nil {
			t.Fatal("delivery fixture intent unreadable")
		}
		var intent struct{ URL string }
		if json.Unmarshal(plain, &intent) != nil {
			t.Fatal("delivery fixture intent invalid")
		}
		link, err := url.Parse(intent.URL)
		if err != nil {
			t.Fatal("delivery fixture link invalid")
		}
		return row.ID, account.ID, link.Query().Get("token")
	}
	sender, err := maildelivery.NewResend("re_artificial_key", "EuroRefurb <mail@example.invalid>", "reply@example.invalid")
	if err != nil {
		t.Fatal("artificial sender config failed")
	}
	oracle := &deliveryOracle{sender: sender}
	calls := 0
	var keys []string
	var bodies [][]byte
	oracle.send = func(_ context.Context, key string, body []byte) maildelivery.Result {
		calls++
		keys = append(keys, key)
		bodies = append(bodies, append([]byte(nil), body...))
		return maildelivery.Result{ID: "00000000-0000-4000-8000-000000000099"}
	}
	id, _, token := fixture()
	done, err := store.DispatchVerification(ctx, oracle)
	if err != nil || done.ID != id || done.Status != "sent" || done.Attempts != 1 || calls != 1 || keys[0] != "mail/"+id || !bytes.Contains(bodies[0], []byte(token)) {
		t.Fatalf("verification delivery failed: error=%v state=%s attempts=%d calls=%d job_match=%t", err, done.Status, done.Attempts, calls, done.ID == id)
	}
	empty, err := store.DispatchVerification(ctx, oracle)
	if err != nil || empty.ID != "" || calls != 1 {
		t.Fatal("sent verification mail replayed")
	}
	var persisted struct {
		Payload, RequestPayload []byte
		ProviderID              string
		Attempts                uint
	}
	if db.Raw("SELECT payload,request_payload,provider_id,attempts FROM customer_verification_mail WHERE id=?", id).Scan(&persisted).Error != nil || bytes.Contains(persisted.RequestPayload, []byte(token)) || bytes.Contains(persisted.Payload, []byte(token)) || persisted.Attempts != 1 || persisted.ProviderID == "" {
		t.Fatal("delivery request not encrypted or provider observation absent")
	}
	for _, mode := range []string{"expired", "verified", "disabled", "replaced"} {
		id, account, token := fixture()
		switch mode {
		case "expired":
			exec("UPDATE customer_verifications SET expires_at=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE customer_id=?", account)
		case "verified":
			exec("UPDATE customer_accounts SET verified_at=UTC_TIMESTAMP(6) WHERE id=?", account)
		case "disabled":
			exec("UPDATE customer_accounts SET enabled=FALSE WHERE id=?", account)
		case "replaced":
			exec("UPDATE customer_verifications SET token_hash=? WHERE customer_id=?", bytes.Repeat([]byte{9}, 32), account)
		}
		before := calls
		result, err := store.DispatchVerification(ctx, oracle)
		if err != nil || result.ID != id || result.Status != "cancelled" || calls != before {
			t.Fatal("inactive verification attempted delivery")
		}
		_ = token
	}
	// A process crash after provider acceptance but before SQL recording retries
	// exactly the same encrypted payload and key, never a newly rendered request.
	id, _, _ = fixture()
	before := calls
	oracle.send = func(_ context.Context, key string, body []byte) maildelivery.Result {
		calls++
		keys = append(keys, key)
		bodies = append(bodies, append([]byte(nil), body...))
		if calls == before+1 {
			exec("CREATE TRIGGER customer_mail_finish_failure BEFORE UPDATE ON customer_verification_mail FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial finish failure'")
		}
		return maildelivery.Result{ID: "00000000-0000-4000-8000-000000000099"}
	}
	if _, err := store.DispatchVerification(ctx, oracle); !errors.Is(err, customer.ErrUnavailable) {
		t.Fatal("failed result recording returned success")
	}
	exec("DROP TRIGGER customer_mail_finish_failure")
	exec("UPDATE customer_verification_mail SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE id=?", id)
	exec("UPDATE customer_mail_dispatch_gate SET next_claim_at='2000-01-01' WHERE id=1")
	result, err := store.DispatchVerification(ctx, oracle)
	if err != nil || result.Status != "sent" || result.Attempts != 2 || calls != before+2 || keys[len(keys)-1] != keys[len(keys)-2] || !bytes.Equal(bodies[len(bodies)-1], bodies[len(bodies)-2]) {
		t.Fatal("ambiguous send retry changed request or key")
	}
	// Temporary provider errors retry at most three times with one-minute delay.
	id, _, _ = fixture()
	before = calls
	oracle.send = func(_ context.Context, key string, body []byte) maildelivery.Result {
		calls++
		return maildelivery.Result{Code: "network_error", Retry: true}
	}
	for attempt := 1; attempt <= 3; attempt++ {
		result, err = store.DispatchVerification(ctx, oracle)
		wanted := "pending"
		if attempt == 3 {
			wanted = "failed"
		}
		if err != nil || result.Status != wanted || result.Attempts != uint(attempt) {
			t.Fatal("bounded provider retry state incorrect")
		}
		if attempt == 1 {
			empty, err := store.DispatchVerification(ctx, oracle)
			if err != nil || empty.ID != "" || calls != before+1 {
				t.Fatal("retry delay ignored")
			}
		}
		exec("UPDATE customer_verification_mail SET next_attempt_at=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE id=?", id)
		exec("UPDATE customer_mail_dispatch_gate SET next_claim_at='2000-01-01' WHERE id=1")
	}
	empty, err = store.DispatchVerification(ctx, oracle)
	if err != nil || empty.ID != "" || calls != before+3 {
		t.Fatal("failed delivery automatically retried")
	}
	// Sender/key changes after an uncertain request cannot silently move the
	// same idempotency key to another Resend account or change the message body.
	id, _, _ = fixture()
	if _, err := store.DispatchVerification(ctx, oracle); err != nil {
		t.Fatal("sender-change fixture failed")
	}
	changed, _ := maildelivery.NewResend("re_other_artificial_key", "mail@example.invalid", "")
	oracle.sender = changed
	before = calls
	exec("UPDATE customer_verification_mail SET next_attempt_at=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE id=?", id)
	exec("UPDATE customer_mail_dispatch_gate SET next_claim_at='2000-01-01' WHERE id=1")
	result, err = store.DispatchVerification(ctx, oracle)
	if err != nil || result.Status != "failed" || calls != before {
		t.Fatal("sender changed across ambiguous replay")
	}
	oracle.sender = sender
	// Two distinct jobs still share the database pacing gate.
	_, _, _ = fixture()
	if _, err := store.Register(ctx, customer.Registration{Email: "delivery-paced@example.invalid", Password: "fixture", FirstName: "Paced", LastName: "Customer", Country: "dk", Locale: "en"}); err != nil {
		t.Fatal("paced second customer fixture failed")
	}
	before = calls
	oracle.send = func(_ context.Context, key string, body []byte) maildelivery.Result {
		calls++
		return maildelivery.Result{ID: "00000000-0000-4000-8000-000000000099"}
	}
	if _, err := store.DispatchVerification(ctx, oracle); err != nil {
		t.Fatal("first paced job failed")
	}
	empty, err = store.DispatchVerification(ctx, oracle)
	if err != nil || empty.ID != "" || calls != before+1 {
		t.Fatal("cross-job global pacing gate ignored")
	}
	// Explicit provider rejection is terminal. It does not silently retry.
	id, _, _ = fixture()
	oracle.send = func(_ context.Context, key string, body []byte) maildelivery.Result {
		calls++
		return maildelivery.Result{Code: "provider_rejected"}
	}
	result, err = store.DispatchVerification(ctx, oracle)
	if err != nil || result.Status != "failed" || result.Attempts != 1 {
		t.Fatal("permanent provider rejection not terminal")
	}
	// Never retry past the original provider idempotency safety window, even if
	// damaged external state extended the verification expiry.
	id, _, _ = fixture()
	exec("UPDATE customer_verification_mail SET first_attempt_at=UTC_TIMESTAMP(6)-INTERVAL 24 HOUR,attempts=1 WHERE id=?", id)
	before = calls
	result, err = store.DispatchVerification(ctx, oracle)
	if err != nil || result.Status != "failed" || calls != before {
		t.Fatal("provider retry safety window ignored")
	}
	// Multiple workers select the same intent but only one owns its live lease.
	id, _, _ = fixture()
	before = calls
	var mutex sync.Mutex
	oracle.send = func(_ context.Context, key string, body []byte) maildelivery.Result {
		mutex.Lock()
		calls++
		mutex.Unlock()
		return maildelivery.Result{ID: "00000000-0000-4000-8000-000000000099"}
	}
	var group sync.WaitGroup
	outcomes := make(chan error, 8)
	for range 8 {
		group.Add(1)
		go func() { defer group.Done(); _, err := store.DispatchVerification(ctx, oracle); outcomes <- err }()
	}
	group.Wait()
	close(outcomes)
	for err := range outcomes {
		if err != nil {
			t.Fatal("concurrent mail worker failed")
		}
	}
	if calls != before+1 {
		t.Fatal("concurrent mail workers submitted twice")
	}
	// Verification while a provider request is in flight cancels the durable
	// intent, so a late provider success cannot resurrect it.
	id, _, token = fixture()
	oracle.send = func(_ context.Context, key string, body []byte) maildelivery.Result {
		if err := store.Verify(ctx, token); err != nil {
			t.Fatal("in-flight verify failed")
		}
		return maildelivery.Result{ID: "00000000-0000-4000-8000-000000000099"}
	}
	result, err = store.DispatchVerification(ctx, oracle)
	if err != nil || result.Status != "superseded" {
		t.Fatal("late provider result resurrected cancelled mail")
	}
	// Missing decryption keys fail without changing queue state or contacting a provider.
	_, _, _ = fixture()
	wrongRing, _ := protected.New("wrong", map[string][]byte{"wrong": bytes.Repeat([]byte{12}, 32)})
	wrong, _ := customer.NewVerificationDelivery(db, wrongRing, "https://shop.example")
	if _, err := wrong.DispatchVerification(ctx, oracle); !errors.Is(err, customer.ErrUnavailable) {
		t.Fatal("unreadable verification payload accepted")
	}
	exec("UPDATE customer_verification_mail SET status='cancelled' WHERE status='pending'")
	t.Log(`CUSTOMER_DELIVERY_REHEARSAL {"real_mysql":true,"synthetic_sender":true,"actual_external_email":false,"current_verification_only":true,"expired_verified_disabled_replaced_skipped":true,"encrypted_exact_request":true,"stable_provider_key":true,"ambiguous_result_recovery":true,"bounded_attempts":3,"retry_delay":true,"cross_job_global_pacing":true,"permanent_rejection_terminal":true,"provider_retry_window":true,"changed_sender_denied":true,"concurrent_workers":8,"single_provider_call":true,"late_result_cannot_resurrect":true,"missing_key_fail_closed":true,"admin_configuration_ui":false,"production_worker_enabled":false}`)
}
