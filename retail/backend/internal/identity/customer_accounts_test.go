package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// Runs inside the existing owned, disconnected MySQL/Redis rehearsal. Secrets
// below are artificial and the verification token is never logged or returned
// through an application endpoint.
func checkCustomerAccounts(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store) {
	t.Helper()
	ring, _ := protected.New("fixture", map[string][]byte{"fixture": bytes.Repeat([]byte{71}, 32)})
	store, err := customer.New(db, ring, sessions, "https://shop.example")
	if err != nil {
		t.Fatal("customer factory failed")
	}
	input := customer.Registration{Email: " Same@Example.invalid ", Password: "artificial-new-customer-password", FirstName: "新", LastName: "Customer", Phone: "+45 123", Country: "dk", Locale: "zh"}
	exec := func(query string, args ...any) {
		t.Helper()
		if db.WithContext(ctx).Exec(query, args...).Error != nil {
			t.Fatal("customer fixture SQL failed")
		}
	}
	// A historical same-email subject remains independent even after verification.
	exec("INSERT INTO historical_customer_subjects(id,email,legacy_had_account,created_at,updated_at) VALUES('old-customer-same-email','same@example.invalid',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	a, err := store.Register(ctx, input)
	if err != nil || a.ID == "old-customer-same-email" || a.Email != "same@example.invalid" || a.VerifiedAt != nil {
		t.Fatal("new pending customer registration failed")
	}
	if _, err := store.Login(ctx, a.Email, input.Password); !errors.Is(err, customer.ErrUnverified) {
		t.Fatal("unverified login granted")
	}
	if _, err := store.Register(ctx, input); !errors.Is(err, customer.ErrExists) {
		t.Fatal("duplicate registration overwrote account")
	}
	if _, err := store.Login(ctx, a.Email, "wrong"); !errors.Is(err, customer.ErrCredentials) {
		t.Fatal("wrong customer password accepted")
	}
	readToken := func() string {
		t.Helper()
		var job struct {
			ID                   string
			Payload              []byte
			ExpiresAt, CreatedAt time.Time
		}
		if db.Raw("SELECT id,payload,expires_at,created_at FROM customer_verification_mail WHERE customer_id=? AND status='pending'", a.ID).Scan(&job).Error != nil || job.ID == "" || job.ExpiresAt.Sub(job.CreatedAt) != customer.VerificationTTL {
			t.Fatal("durable verification job missing")
		}
		plain, err := ring.Open(protected.Context{Purpose: "customer-verification", Table: "customer_verification_mail", Column: "payload", Row: job.ID}, job.Payload)
		if err != nil {
			t.Fatal("row-bound verification intent unreadable")
		}
		var message struct{ Email, URL, Locale string }
		if json.Unmarshal(plain, &message) != nil || message.Email != a.Email || message.Locale != "zh" {
			t.Fatal("verification intent projection changed")
		}
		parsed, err := url.Parse(message.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host != "shop.example" || parsed.Path != "/dk-zh/verify-account" {
			t.Fatal("verification link origin changed")
		}
		token := parsed.Query().Get("token")
		if len(token) != 64 || bytes.Contains(job.Payload, []byte(token)) {
			t.Fatal("verification bearer plaintext persisted")
		}
		digest := sha256.Sum256([]byte(token))
		var count int64
		if db.Raw("SELECT COUNT(*) FROM customer_verifications WHERE customer_id=? AND token_hash=?", a.ID, digest[:]).Scan(&count).Error != nil || count != 1 {
			t.Fatal("verification digest not bound to customer")
		}
		return token
	}
	old := readToken()
	if err := store.Resend(ctx, a.Email, "wrong", "dk", "zh"); !errors.Is(err, customer.ErrCredentials) {
		t.Fatal("unauthenticated resend accepted")
	}
	if err := store.Resend(ctx, a.Email, input.Password, "dk", "zh"); !errors.Is(err, customer.ErrRateLimited) {
		t.Fatal("verification resend cooldown ignored")
	}
	exec("UPDATE customer_verifications SET created_at=UTC_TIMESTAMP(6)-INTERVAL 61 SECOND WHERE customer_id=?", a.ID)
	if err := store.Resend(ctx, a.Email, input.Password, "dk", "zh"); err != nil {
		t.Fatal("verified-password resend failed")
	}
	token := readToken()
	if token == old || !errors.Is(store.Verify(ctx, old), customer.ErrVerification) {
		t.Fatal("resend did not invalidate old token")
	}
	exec("UPDATE customer_verifications SET expires_at=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE customer_id=?", a.ID)
	if err := store.Verify(ctx, token); !errors.Is(err, customer.ErrVerification) {
		t.Fatal("expired token accepted")
	}
	exec("UPDATE customer_verifications SET created_at=UTC_TIMESTAMP(6)-INTERVAL 61 SECOND WHERE customer_id=?", a.ID)
	if err := store.Resend(ctx, a.Email, input.Password, "dk", "zh"); err != nil {
		t.Fatal("expired challenge could not be renewed")
	}
	token = readToken()
	exec("CREATE TRIGGER customer_verify_fixture_failure BEFORE UPDATE ON customer_accounts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial late verification failure'")
	if err := store.Verify(ctx, token); !errors.Is(err, customer.ErrUnavailable) {
		t.Fatal("late verification failure accepted")
	}
	exec("DROP TRIGGER customer_verify_fixture_failure")
	var group sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		group.Add(1)
		go func() { defer group.Done(); results <- store.Verify(ctx, token) }()
	}
	group.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, customer.ErrVerification) {
			t.Fatal("concurrent verification storage failure")
		}
	}
	if successes != 1 {
		t.Fatal("verification did not have exactly one winner")
	}
	login, err := store.Login(ctx, " SAME@EXAMPLE.INVALID ", input.Password)
	if err != nil || login.Customer.ID != a.ID || login.Customer.VerifiedAt == nil || len(login.Session.Token) != 64 {
		t.Fatal("verified customer login failed")
	}
	if _, err := sessions.Resolve(ctx, sessionstore.Admin, login.Session.Token); err == nil {
		t.Fatal("customer session accepted by admin realm")
	}
	current, err := store.Current(ctx, login.Session.Token)
	if err != nil || current.ID != a.ID {
		t.Fatal("customer current session failed")
	}
	checkCustomerProfile(t, ctx, db, store, login.Session.Token, a.ID)
	checkCustomerNotices(t, ctx, db, store, login.Session.Token, a.ID)
	checkCustomerAddresses(t, ctx, db, store, sessions, login.Session.Token, a.ID)
	checkCustomerDelivery(t, ctx, db, store, ring)
	public, _ := json.Marshal(login)
	for _, secret := range []string{input.Password, login.Session.Token, login.Session.CSRF, "password_hash", "auth_version", token} {
		if bytes.Contains(public, []byte(secret)) {
			t.Fatal("customer public projection leaked a secret")
		}
	}
	exec("UPDATE customer_accounts SET auth_version=auth_version+1 WHERE id=?", a.ID)
	if _, err := store.UpdateProfile(ctx, login.Session.Token, map[string]string{"phone": "revoked"}); !errors.Is(err, customer.ErrUnauthorized) {
		t.Fatal("revoked customer updated profile")
	}
	if _, err := store.Addresses(ctx, login.Session.Token); !errors.Is(err, customer.ErrUnauthorized) {
		t.Fatal("revoked customer accessed addresses")
	}
	if _, err := store.Current(ctx, login.Session.Token); !errors.Is(err, customer.ErrUnauthorized) {
		t.Fatal("durable customer revocation ignored")
	}
	login, err = store.Login(ctx, a.Email, input.Password)
	if err != nil {
		t.Fatal("new generation login failed")
	}
	adminSession, err := sessions.Issue(ctx, sessionstore.Admin, sessionstore.Principal{UserID: a.ID, Version: strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal("artificial administrator realm unavailable")
	}
	if _, err := store.Current(ctx, adminSession.Token); !errors.Is(err, customer.ErrUnauthorized) {
		t.Fatal("administrator token accepted as customer")
	}
	if err := store.Logout(ctx, login.Session.Token); err != nil {
		t.Fatal("customer logout failed")
	}
	if _, err := store.Current(ctx, login.Session.Token); !errors.Is(err, customer.ErrUnauthorized) {
		t.Fatal("logged-out session accepted")
	}
	login, err = store.Login(ctx, a.Email, input.Password)
	if err != nil {
		t.Fatal("pre-disable customer login failed")
	}
	exec("UPDATE customer_accounts SET enabled=FALSE WHERE id=?", a.ID)
	if _, err := store.Current(ctx, login.Session.Token); !errors.Is(err, customer.ErrUnauthorized) {
		t.Fatal("disabled customer session remained valid")
	}
	if _, err := store.Login(ctx, a.Email, input.Password); !errors.Is(err, customer.ErrCredentials) {
		t.Fatal("disabled customer signed in")
	}
	var oldEmail string
	if db.Raw("SELECT email FROM historical_customer_subjects WHERE id='old-customer-same-email'").Scan(&oldEmail).Error != nil || oldEmail != a.Email {
		t.Fatal("historical subject was changed")
	}
	// A late insert error must roll back identity and challenge, too.
	exec("CREATE TRIGGER customer_mail_fixture_failure BEFORE INSERT ON customer_verification_mail FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial late failure'")
	failing := input
	failing.Email = "rollback@example.invalid"
	if _, err := store.Register(ctx, failing); !errors.Is(err, customer.ErrUnavailable) {
		t.Fatal("late mail failure accepted")
	}
	exec("DROP TRIGGER customer_mail_fixture_failure")
	var count int64
	if db.Raw("SELECT COUNT(*) FROM customer_accounts WHERE email=?", failing.Email).Scan(&count).Error != nil || count != 0 {
		t.Fatal("failed registration left an identity")
	}
	if db.Raw("SELECT COUNT(*) FROM customer_verifications WHERE customer_id NOT IN (SELECT id FROM customer_accounts)").Scan(&count).Error != nil || count != 0 {
		t.Fatal("failed registration left a challenge")
	}
	if _, err := store.Current(ctx, strings.Repeat("f", 64)); !errors.Is(err, customer.ErrUnauthorized) {
		t.Fatal("unknown customer session granted")
	}
	checkCustomerHTTP(t, ctx, db, sessions, ring)
	t.Log(`CUSTOMER_ACCOUNTS_REHEARSAL {"new_customer_only":true,"email_normalized":true,"unverified_login_denied":true,"duplicate_cannot_overwrite":true,"password_gated_resend":true,"sixty_second_resend_cooldown":true,"encrypted_mail_intent":true,"token_digest_only":true,"fifteen_minute_expiry":true,"resend_invalidates_old_link":true,"concurrent_verify_attempts":8,"single_consumption":true,"customer_admin_realms_separate":true,"public_projection_safe":true,"durable_revocation":true,"logout_revocation":true,"disabled_login_denied":true,"same_email_history_unchanged":true,"late_failure_atomic":true,"late_verification_failure_atomic":true,"http_integrated":true,"mail_delivery_integrated":false,"storefront_integrated":false}`)
}
