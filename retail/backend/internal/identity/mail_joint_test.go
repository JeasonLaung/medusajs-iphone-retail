package identity_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/eurorefurb/retail/internal/notificationmail"
	"github.com/eurorefurb/retail/internal/protected"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type jointMailTransport func(*http.Request) (*http.Response, error)

func (f jointMailTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Explicit isolated rehearsal only. The production Gin API and browser create
// the mailbox and registration. Only the external provider transport is replaced
// in this short-lived test process; no real recipients or credentials are used.
func TestOwnedMailBrowserWorker(t *testing.T) {
	if os.Getenv("BROWSER_IDENTITY_FIXTURE") != "1" || os.Getenv("MAIL_JOINT_PHASE") == "" {
		t.Skip("owned mail browser fixture required")
	}
	c := driver.NewConfig()
	c.User, c.Passwd, c.Net, c.Addr, c.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "browser_identity"
	c.ParseTime, c.Loc = true, time.UTC
	db, err := gorm.Open(mysql.Open(c.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("owned mail SQL unavailable")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal("owned mail pool unavailable")
	}
	defer pool.Close()
	ring, err := protected.ParseKeyring(os.Getenv("MERCHANT_KEY_ACTIVE"), os.Getenv("MERCHANT_KEYRING_JSON"))
	if err != nil {
		t.Fatal("owned mail keyring unavailable")
	}
	mailbox := notificationmail.Store{DB: db, Ring: ring}
	ctx := t.Context()
	const job = "00000000-0000-4000-8000-000000000092"
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("owned mail fixture SQL failed")
		}
	}
	switch os.Getenv("MAIL_JOINT_PHASE") {
	case "verification":
		previous := http.DefaultTransport
		defer func() { http.DefaultTransport = previous }()
		calls, link := 0, ""
		http.DefaultTransport = jointMailTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.String() != "https://api.resend.com/emails" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer re_joint_artificial_key" || !strings.HasPrefix(r.Header.Get("Idempotency-Key"), "mail/cv_") {
				t.Fatal("verification provider request differs")
			}
			var body struct {
				From, ReplyTo, Text string
				To                  []string
			}
			raw, e := io.ReadAll(r.Body)
			var fields map[string]json.RawMessage
			if e != nil || json.Unmarshal(raw, &fields) != nil || json.Unmarshal(raw, &body) != nil {
				t.Fatal("verification request body invalid")
			}
			if json.Unmarshal(fields["reply_to"], &body.ReplyTo) != nil || !strings.Contains(body.From, "joint-sender@example.invalid") || body.ReplyTo != "joint-reply@example.invalid" || len(body.To) != 1 || body.To[0] != "joint-customer@example.invalid" {
				t.Fatal("verification ignored browser mailbox configuration")
			}
			for _, line := range strings.Split(body.Text, "\n") {
				if strings.HasPrefix(line, "http://127.0.0.1:19383/dk-en/verify-account?token=") {
					link = line
				}
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"00000000-0000-4000-8000-000000000091"}`))}, nil
		})
		sender, e := mailbox.RegistrationSender(ctx)
		customerRing, ce := protected.ParseKeyring(os.Getenv("CUSTOMER_KEY_ACTIVE"), os.Getenv("CUSTOMER_KEYRING_JSON"))
		if e != nil || ce != nil {
			t.Fatal("browser-selected sender unavailable")
		}
		store, e := customer.NewVerificationDelivery(db, customerRing, "http://127.0.0.1:19383")
		if e != nil {
			t.Fatal("verification worker unavailable")
		}
		out, e := store.DispatchVerification(ctx, sender)
		if e != nil || out.Status != "sent" || out.Attempts != 1 || calls != 1 || link == "" {
			t.Fatal("browser registration dispatch failed")
		}
		if empty, e := store.DispatchVerification(ctx, sender); e != nil || empty.ID != "" || calls != 1 {
			t.Fatal("sent verification dispatched twice")
		}
		if os.WriteFile("/tmp/joint-mail-link.json", []byte(mustJointJSON(t, map[string]string{"link": link})), 0600) != nil {
			t.Fatal("private fixture link transfer failed")
		}
		// Artificial failed order mail for browser recovery. This does not claim
		// a completed payment or order-worker dispatch.
		sender, e = mailbox.OrderSender(ctx)
		if e != nil {
			t.Fatal("order sender unavailable")
		}
		body, e := sender.PrepareOrder(maildelivery.OrderPlaced{To: "joint-customer@example.invalid", Reference: "9007199254740993123", Amount: "158.75", Currency: "eur", Locale: "en"})
		if e != nil {
			t.Fatal("order body unavailable")
		}
		sealed, e := ring.Seal(protected.Context{Purpose: "order-mail-request-v1", Table: "order_mail_deliveries", Column: "request_envelope", Row: job}, body)
		if e != nil {
			t.Fatal("order body encryption failed")
		}
		exec("INSERT INTO order_notification_outbox(id,order_id,event,payload_envelope,status,attempts,created_at) VALUES(?,'read-order','order.placed','artificial-held-intent','failed',3,UTC_TIMESTAMP(6))", job)
		exec("INSERT INTO order_mail_deliveries(notification_id,first_attempt_at,error_code,request_envelope,sender_fingerprint,failed_at) VALUES(?,UTC_TIMESTAMP(6),'network_error',?,?,UTC_TIMESTAMP(6))", job, sealed, sender.Fingerprint())
	case "recovered":
		var row struct {
			Status            string
			Attempts          uint
			RequestEnvelope   []byte
			SenderFingerprint string
		}
		if db.Raw("SELECT n.status,n.attempts,d.request_envelope,d.sender_fingerprint FROM order_notification_outbox n JOIN order_mail_deliveries d ON d.notification_id=n.id WHERE n.id=?", job).Scan(&row).Error != nil || row.Status != "pending" || row.Attempts != 0 {
			t.Fatal("browser retry not durable")
		}
		plain, e := ring.Open(protected.Context{Purpose: "order-mail-request-v1", Table: "order_mail_deliveries", Column: "request_envelope", Row: job}, row.RequestEnvelope)
		sender, se := mailbox.OrderSender(ctx)
		if e != nil || se != nil || row.SenderFingerprint != sender.Fingerprint() || !bytes.Contains(plain, []byte("158.75 EUR")) {
			t.Fatal("browser recovery changed frozen mail")
		}
		var count int64
		if db.Table("audit_events").Where("action='order_mail.retry' AND resource_id=?", job).Count(&count).Error != nil || count != 1 {
			t.Fatal("browser recovery audit missing")
		}
		if db.Table("customer_accounts").Where("email=? AND verified_at IS NOT NULL", "joint-customer@example.invalid").Count(&count).Error != nil || count != 1 {
			t.Fatal("browser verification not durable")
		}
		if db.Table("notification_mail_commands").Count(&count).Error != nil || count != 2 {
			t.Fatal("browser mailbox create/edit not durable")
		}
		var secret struct{ APIKey []byte }
		if db.Table("mail_accounts").Select("api_key").Where("email=?", "joint-sender@example.invalid").Take(&secret).Error != nil || bytes.Contains(secret.APIKey, []byte("re_joint_artificial_key")) {
			t.Fatal("mail secret not encrypted")
		}
	default:
		t.Fatal("unsupported owned mail phase")
	}
	t.Log("OWNED_MAIL_JOINT_PHASE_PASSED")
}

func mustJointJSON(t *testing.T, in any) string {
	t.Helper()
	b, e := json.Marshal(in)
	if e != nil {
		t.Fatal("fixture encoding failed")
	}
	return string(b)
}
