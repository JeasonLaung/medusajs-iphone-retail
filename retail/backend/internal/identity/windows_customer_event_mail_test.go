package identity_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/customermail"
	"github.com/eurorefurb/retail/internal/customernotices"
	"github.com/eurorefurb/retail/internal/notificationmail"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWindowsCustomerEventMail(t *testing.T) {
	if os.Getenv("WINDOWS_MAIL_SUBFLOW") != "1" {
		t.Skip("owned event mail fixture required")
	}
	cfg, e := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	owner, logs := os.Getenv("ACCOUNT_TEST_OWNER"), os.Getenv("ACCOUNT_TEST_LOGS")
	if e != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(owner) || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13307" || cfg.DBName != "retail_account_"+owner || filepath.Base(logs) != "account-"+owner {
		t.Fatal("owned loopback event mail database required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal("database unavailable")
	}
	pool, _ := db.DB()
	defer pool.Close()
	ring, e := protected.ParseKeyring(os.Getenv("CUSTOMER_KEY_ACTIVE"), os.Getenv("CUSTOMER_KEYRING_JSON"))
	if e != nil {
		t.Fatal("customer ring unavailable")
	}
	merchant, e := protected.ParseKeyring(os.Getenv("MERCHANT_KEY_ACTIVE"), os.Getenv("MERCHANT_KEYRING_JSON"))
	if e != nil {
		t.Fatal("merchant ring unavailable")
	}
	mailbox := notificationmail.Store{DB: db, Ring: merchant}
	sender, e := mailbox.OrderSender(t.Context())
	if e != nil {
		t.Fatal("sender unavailable")
	}
	store := customermail.Store{DB: db, Ring: ring}
	phase := os.Getenv("WINDOWS_MAIL_PHASE")
	recipientOwner := os.Getenv("WINDOWS_MAIL_RECIPIENT_OWNER")
	if recipientOwner == "" {
		recipientOwner = owner
	}
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(recipientOwner) {
		t.Fatal("owned recipient required")
	}
	resource := os.Getenv("WINDOWS_MAIL_RESOURCE")
	if resource != "" && !regexp.MustCompile(`^(bbr|order)_[0-9a-f-]{36}$`).MatchString(resource) {
		t.Fatal("owned mail resource required")
	}
	var calls atomic.Int32
	prior := http.DefaultTransport
	defer func() { http.DefaultTransport = prior }()
	// Construct sender after installing the transport; the real sender retains it.
	http.DefaultTransport = jointMailTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.String() != "https://api.resend.com/emails" || r.Header.Get("Authorization") != "Bearer re_windows_isolated_mail_key" {
			return nil, errors.New("unexpected provider request")
		}
		body, e := io.ReadAll(r.Body)
		if e != nil {
			return nil, e
		}
		var message struct {
			To            []string
			Subject, Text string
		}
		if json.Unmarshal(body, &message) != nil || len(message.To) != 1 || message.To[0] != "local-account-"+recipientOwner+"-1@example.invalid" || message.Subject == "" {
			return nil, errors.New("wrong event recipient or resource")
		}
		key := r.Header.Get("Idempotency-Key")
		id, e := uuid.Parse(strings.TrimPrefix(key, "mail/ce_"))
		if e != nil || key != "mail/ce_"+id.String() {
			return nil, errors.New("wrong event key")
		}
		var event struct{ ResourceID, Reference string }
		if db.Table("customer_event_mail").Where("id=?", id.String()).Take(&event).Error != nil || resource != "" && event.ResourceID != resource || !strings.Contains(message.Text, event.Reference) {
			return nil, errors.New("wrong resource reference")
		}
		if phase == "event-reject" {
			if os.WriteFile(filepath.Join(logs, "frozen-event-mail.json"), body, 0600) != nil || os.WriteFile(filepath.Join(logs, "frozen-event-key.txt"), []byte(key), 0600) != nil {
				return nil, errors.New("evidence unavailable")
			}
			return &http.Response{StatusCode: 422, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"name":"validation_error"}`))}, nil
		}
		if phase == "event-sent" {
			old, e := os.ReadFile(filepath.Join(logs, "frozen-event-mail.json"))
			oldKey, _ := os.ReadFile(filepath.Join(logs, "frozen-event-key.txt"))
			if e != nil || !bytes.Equal(old, body) || string(oldKey) != key {
				return nil, errors.New("retry changed message or key")
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"00000000-0000-4000-8000-000000000099"}`))}, nil
	})
	sender, e = mailbox.OrderSender(t.Context())
	if e != nil {
		t.Fatal("sender unavailable")
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("fixture SQL")
		}
	}
	exec("UPDATE customer_event_mail_gate SET next_claim_at='1970-01-01' WHERE id=1")
	switch phase {
	case "event-reject", "event-sent":
		var before int64
		db.Table("customer_event_mail").Where("status='sent'").Count(&before)
		var statuses [6]string
		var wg sync.WaitGroup
		var failures atomic.Int32
		for i := range statuses {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				out, e := store.Dispatch(t.Context(), sender)
				if e != nil {
					failures.Add(1)
				}
				statuses[i] = out.Status
			}(i)
		}
		wg.Wait()
		want := "sent"
		if phase == "event-reject" {
			want = "failed"
		}
		n := 0
		for _, status := range statuses {
			if status == want {
				n++
			} else if status != "" {
				t.Fatalf("unexpected status %s", status)
			}
		}
		if failures.Load() != 0 || n != 1 || calls.Load() != 1 {
			t.Fatalf("concurrent claim differs successes=%d calls=%d failures=%d", n, calls.Load(), failures.Load())
		}
		if phase == "event-sent" {
			var after int64
			db.Table("customer_event_mail").Where("status='sent'").Count(&after)
			if after != before+1 {
				t.Fatal("sent count differs")
			}
		}
	case "event-drain":
		// Exercise stage idempotency and rollback without creating fixture business data.
		var job struct{ CustomerID, ResourceID string }
		if db.Table("customer_event_mail").Take(&job).Error != nil {
			t.Fatal("committed event missing")
		}
		key := "rollback:" + uuid.NewString()
		stop := errors.New("rollback fixture")
		err := db.Transaction(func(tx *gorm.DB) error {
			for range 2 {
				if e := customernotices.StageMailTx(tx, job.CustomerID, key, "buyback.updated", job.ResourceID, job.ResourceID, "mark_paid", time.Now().UTC()); e != nil {
					return e
				}
			}
			var count int64
			tx.Table("customer_event_mail").Where("event_key=?", key).Count(&count)
			if count != 1 {
				return errors.New("event replay duplicated")
			}
			return stop
		})
		if !errors.Is(err, stop) {
			t.Fatal("transaction replay failed")
		}
		var count int64
		db.Table("customer_event_mail").Where("event_key=?", key).Count(&count)
		if count != 0 {
			t.Fatal("failed transaction left mail")
		}
		for range 30 {
			exec("UPDATE customer_event_mail_gate SET next_claim_at='1970-01-01' WHERE id=1")
			out, e := store.Dispatch(t.Context(), sender)
			if e != nil {
				t.Fatal("event drain failed")
			}
			if out.ID == "" {
				break
			}
			if out.Status != "sent" {
				t.Fatalf("event drain state %s", out.Status)
			}
		}
		db.Table("customer_event_mail").Where("status<>'sent'").Count(&count)
		if count != 0 {
			t.Fatal("event queue unfinished")
		}
	case "event-safety":
		var job struct{ ID, CustomerID, ResourceID string }
		if db.Table("customer_event_mail").Where("status='sent'").Take(&job).Error != nil {
			t.Fatal("sent event missing")
		}
		var account struct{ Email string }
		if db.Table("customer_accounts").Where("id=?", job.CustomerID).Take(&account).Error != nil {
			t.Fatal("account missing")
		}
		// The sent request is preserved; each guard must stop before any transport.
		check := func(code string) {
			t.Helper()
			exec("UPDATE customer_event_mail_gate SET next_claim_at='1970-01-01' WHERE id=1")
			exec("UPDATE customer_event_mail SET status='pending',attempts=0 WHERE id=?", job.ID)
			out, e := store.Dispatch(t.Context(), sender)
			if e != nil || out.Status != "failed" {
				t.Fatalf("guard %s did not stop", code)
			}
			var d struct{ ErrorCode string }
			if db.Table("customer_event_mail_deliveries").Where("notification_id=?", job.ID).Take(&d).Error != nil || d.ErrorCode != code {
				t.Fatalf("guard %s reason differs", code)
			}
		}
		exec("UPDATE customer_accounts SET enabled=FALSE WHERE id=?", job.CustomerID)
		check("recipient_unavailable")
		exec("UPDATE customer_accounts SET enabled=TRUE WHERE id=?", job.CustomerID)
		exec("UPDATE customer_accounts SET email=? WHERE id=?", "changed-"+owner+"@example.invalid", job.CustomerID)
		check("recipient_changed")
		exec("UPDATE customer_accounts SET email=? WHERE id=?", account.Email, job.CustomerID)
		exec("UPDATE customer_event_mail SET resource_id='bbr_missing' WHERE id=?", job.ID)
		check("recipient_changed")
		exec("UPDATE customer_event_mail SET resource_id=? WHERE id=?", job.ResourceID, job.ID)
		exec("UPDATE customer_event_mail_deliveries SET sender_fingerprint=REPEAT('0',64) WHERE notification_id=?", job.ID)
		check("sender_changed")
		exec("UPDATE customer_event_mail_deliveries SET sender_fingerprint=? WHERE notification_id=?", sender.Fingerprint(), job.ID)
		exec("UPDATE customer_event_mail_deliveries SET first_attempt_at=UTC_TIMESTAMP(6)-INTERVAL 24 HOUR WHERE notification_id=?", job.ID)
		check("retry_exhausted")
		// Fixture restores the terminal row after exercising guards; no dispatch follows.
		exec("UPDATE customer_event_mail SET status='sent',attempts=1 WHERE id=?", job.ID)
		exec("UPDATE customer_event_mail_deliveries SET first_attempt_at=sent_at,failed_at=NULL,error_code=NULL WHERE notification_id=?", job.ID)
		if calls.Load() != 0 {
			t.Fatal("guard contacted provider")
		}
	default:
		t.Fatal("unknown event phase")
	}
	raw, _ := json.Marshal(map[string]any{"passed": true, "phase": phase, "external_transport_isolated": true, "provider_calls": calls.Load(), "concurrent_claims": phase == "event-reject" || phase == "event-sent"})
	if os.WriteFile(filepath.Join(logs, phase+"-result.json"), raw, 0600) != nil {
		t.Fatal("evidence write")
	}
}
