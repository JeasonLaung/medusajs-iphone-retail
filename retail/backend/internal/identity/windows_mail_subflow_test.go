package identity_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/notificationmail"
	"github.com/eurorefurb/retail/internal/ordermail"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
	driver "github.com/go-sql-driver/mysql"
)

// A separate, explicitly gated fixture process runs the production dispatchers.
// Only Resend's external transport is replaced; SQL intents, mailbox keys,
// immutable request envelopes and the Admin retry command remain real.
func TestWindowsMailSubflow(t *testing.T) {
	if os.Getenv("WINDOWS_MAIL_SUBFLOW") != "1" {
		t.Skip("explicit owned Windows mail fixture required")
	}
	cfg, e := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	owner, logs := os.Getenv("ACCOUNT_TEST_OWNER"), os.Getenv("ACCOUNT_TEST_LOGS")
	if e != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(owner) || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13307" || cfg.DBName != "retail_account_"+owner || filepath.Base(logs) != "account-"+owner {
		t.Fatal("owned loopback mail database and log directory required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal("owned mail database unavailable")
	}
	pool, _ := db.DB()
	defer pool.Close()
	merchant, e := protected.ParseKeyring(os.Getenv("MERCHANT_KEY_ACTIVE"), os.Getenv("MERCHANT_KEYRING_JSON"))
	if e != nil {
		t.Fatal("mail keyring unavailable")
	}
	mailbox := notificationmail.Store{DB: db, Ring: merchant}
	phase := os.Getenv("WINDOWS_MAIL_PHASE")
	calls := 0
	var sent []byte
	prior := http.DefaultTransport
	defer func() { http.DefaultTransport = prior }()
	http.DefaultTransport = jointMailTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.String() != "https://api.resend.com/emails" || r.Header.Get("Authorization") != "Bearer re_windows_isolated_mail_key" {
			t.Fatal("unexpected external mail request")
		}
		calls++
		sent, e = io.ReadAll(r.Body)
		if e != nil {
			t.Fatal("mail body unavailable")
		}
		var body struct {
			To         []string
			From, Text string
		}
		if json.Unmarshal(sent, &body) != nil || len(body.To) != 1 || !strings.HasSuffix(body.To[0], "@example.invalid") || !strings.Contains(body.From, "windows-notify@example.invalid") {
			t.Fatal("mail sender/recipient mismatch")
		}
		key := r.Header.Get("Idempotency-Key")
		if phase == "verification" {
			if !strings.HasPrefix(key, "mail/cv_") {
				t.Fatal("verification key mismatch")
			}
			link := ""
			for _, line := range strings.Split(body.Text, "\n") {
				if strings.HasPrefix(line, "http://127.0.0.1:19300/gb-en/verify-account?token=") {
					link = line
				}
			}
			if link == "" || os.WriteFile(filepath.Join(logs, "mail-verification-link.txt"), []byte(link), 0600) != nil {
				t.Fatal("verification link missing")
			}
		} else {
			if !strings.HasPrefix(key, "mail/op_") || !bytes.Contains(sent, []byte("789 EUR")) {
				t.Fatal("paid order mail mismatch")
			}
			path := filepath.Join(logs, "frozen-order-mail.json")
			if phase == "order-reject" {
				if os.WriteFile(path, sent, 0600) != nil {
					t.Fatal("mail evidence write")
				}
				return &http.Response{StatusCode: 422, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"name":"validation_error"}`))}, nil
			}
			frozen, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(frozen, sent) {
				t.Fatal("retry changed frozen message")
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"00000000-0000-4000-8000-000000000099"}`))}, nil
	})
	ctx := t.Context()
	if phase == "protocol-smtp" || phase == "protocol-quguo" {
		sender, e := mailbox.RegistrationSender(ctx)
		if e != nil || sender == nil || sender.SupportsIdempotency() {
			t.Fatal("configured protocol was not selected")
		}
		var row struct {
			AccountID      string
			ConfigEnvelope []byte
		}
		if db.Table("notification_mail_transports").Take(&row).Error != nil {
			t.Fatal("encrypted protocol settings missing")
		}
		for _, secret := range []string{"smtp-isolated-secret", "imap-isolated-secret", "relayme-isolated-secret"} {
			if bytes.Contains(row.ConfigEnvelope, []byte(secret)) {
				t.Fatal("protocol credential persisted in plaintext")
			}
		}
		raw, e := merchant.Open(protected.Context{Purpose: "notification-mail-transport", Table: "notification_mail_transports", Column: "config_envelope", Row: row.AccountID}, row.ConfigEnvelope)
		if e != nil {
			t.Fatal("protocol settings cannot be decrypted")
		}
		defer clear(raw)
		var config notificationmail.TransportConfig
		if json.Unmarshal(raw, &config) != nil || config.Provider != strings.TrimPrefix(phase, "protocol-") || config.IMAP == nil || config.IMAP.Password != "imap-isolated-secret" {
			t.Fatal("saved protocol or blank IMAP password preservation differs")
		}
		if phase == "protocol-smtp" && (config.SMTP == nil || config.SMTP.Password != "smtp-isolated-secret") {
			t.Fatal("blank SMTP password did not preserve credential")
		}
		if phase == "protocol-quguo" && (config.Relayme == nil || config.Relayme.Secret != "relayme-isolated-secret") {
			t.Fatal("blank Relayme secret did not preserve credential")
		}
		if calls != 0 {
			t.Fatal("configuration verification sent external mail")
		}
	} else if phase == "verification" {
		ring, e := protected.ParseKeyring(os.Getenv("CUSTOMER_KEY_ACTIVE"), os.Getenv("CUSTOMER_KEYRING_JSON"))
		if e != nil {
			t.Fatal("customer ring unavailable")
		}
		sender, e := mailbox.RegistrationSender(ctx)
		if e != nil {
			t.Fatal("selected registration sender unavailable")
		}
		store, e := customer.NewVerificationDelivery(db, ring, "http://127.0.0.1:19300")
		if e != nil {
			t.Fatal("verification delivery unavailable")
		}
		out, e := store.DispatchVerification(ctx, sender)
		if e != nil || out.Status != "sent" || calls != 1 {
			t.Fatal("verification mail dispatch failed")
		}
		if next, e := store.DispatchVerification(ctx, sender); e != nil || next.ID != "" || calls != 1 {
			t.Fatal("sent verification replayed")
		}
	} else if phase == "order-reject" || phase == "order-sent" {
		ring, e := protected.ParseKeyring(os.Getenv("PAYMENT_KEY_ACTIVE"), os.Getenv("PAYMENT_KEYRING_JSON"))
		if e != nil {
			t.Fatal("order ring unavailable")
		}
		sender, e := mailbox.OrderSender(ctx)
		if e != nil {
			t.Fatal("order sender unavailable")
		}
		store := ordermail.Store{DB: db, Ring: ring}
		out, e := store.Dispatch(ctx, sender)
		want := "sent"
		if phase == "order-reject" {
			want = "failed"
		}
		if e != nil || out.Status != want || out.Attempts != 1 || calls != 1 {
			t.Fatalf("order dispatch status=%s attempts=%d calls=%d error=%v", out.Status, out.Attempts, calls, e)
		}
		if phase == "order-sent" {
			time.Sleep(time.Second)
			if next, e := store.Dispatch(ctx, sender); e != nil || next.ID != "" || calls != 1 {
				t.Fatal("sent order replayed")
			}
		}
	} else {
		t.Fatal("unknown phase")
	}
	if os.WriteFile(filepath.Join(logs, "mail-"+phase+"-passed.txt"), []byte("production dispatcher; isolated Resend transport; calls=1"), 0600) != nil {
		t.Fatal("phase evidence write")
	}
}
