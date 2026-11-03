package identity_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap/backend/memory"
	imapserver "github.com/emersion/go-imap/server"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/eurorefurb/retail/internal/notificationmail"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// Only the remote IMAP server is synthetic; configuration, synchronizer,
// archive encryption, browser routes and mailbox state are production code.
func TestWindowsInboxBrowser(t *testing.T) {
	if os.Getenv("WINDOWS_MAIL_SUBFLOW") != "1" {
		t.Skip("owned Windows mail fixture required")
	}
	cfg, e := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	owner, logs := os.Getenv("ACCOUNT_TEST_OWNER"), os.Getenv("ACCOUNT_TEST_LOGS")
	if e != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(owner) || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13307" || cfg.DBName != "retail_account_"+owner || filepath.Base(logs) != "account-"+owner {
		t.Fatal("owned mail DB required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	defer pool.Close()
	ring, e := protected.ParseKeyring(os.Getenv("MERCHANT_KEY_ACTIVE"), os.Getenv("MERCHANT_KEYRING_JSON"))
	if e != nil {
		t.Fatal("merchant ring unavailable")
	}
	var actors []identity.User
	if db.Table("admin_users").Find(&actors).Error != nil {
		t.Fatal("admin fixture missing")
	}
	var actor identity.User
	for _, candidate := range actors {
		if db.Transaction(func(tx *gorm.DB) error {
			return authorization.CheckTx(t.Context(), tx, candidate, authorization.MailWrite)
		}) == nil {
			actor = candidate
			break
		}
	}
	if actor.ID == "" {
		t.Fatal("mail manager missing")
	}
	store := notificationmail.Store{DB: db, Ring: ring}
	directory, e := store.Directory(t.Context(), actor)
	if e != nil || len(directory.Accounts) != 1 {
		t.Fatal("mailbox fixture missing")
	}
	account := directory.Accounts[0]
	callbackRouter := gin.New()
	store.RegisterWebhooks(callbackRouter)
	var submissions atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var message struct {
			To, Cc, Bcc []string
			Subject     string
			Attachments []maildelivery.Attachment
		}
		if r.Method != "POST" || r.URL.Path != "/api/external/v1/emails" || r.Header.Get("Authorization") != "Bearer owned.fixture-secret" || json.NewDecoder(r.Body).Decode(&message) != nil || len(message.To) != 1 || message.To[0] != "compose@example.invalid" || len(message.Cc) != 1 || len(message.Bcc) != 1 || len(message.Attachments) != 1 || message.Attachments[0].Content != "QXR0YWNobWVudCBmaXh0dXJl" {
			w.WriteHeader(422)
			return
		}
		submissions.Add(1)
		early, _ := json.Marshal(map[string]any{"event_id": "early-delay", "event_type": "delivery_status", "occurred_at": time.Now().UTC().Add(-time.Minute), "submission_id": "owned-compose-accepted", "mail_from": account.Email, "recipient": "compose@example.invalid", "status": "delayed"})
		path := "/api/webhooks/mail/" + account.ID + "/relayme"
		stamp := strconv.FormatInt(time.Now().Unix(), 10)
		digest := sha256.Sum256(early)
		mac := hmac.New(sha256.New, []byte("fixture-secret"))
		_, _ = mac.Write([]byte("POST\n" + path + "\n" + stamp + "\nnonce\n" + hex.EncodeToString(digest[:])))
		callback := httptest.NewRequest("POST", path, bytes.NewReader(early))
		callback.Header.Set("X-Relayme-Key-ID", "owned")
		callback.Header.Set("X-Relayme-Timestamp", stamp)
		callback.Header.Set("X-Relayme-Nonce", "nonce")
		callback.Header.Set("X-Relayme-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
		response := httptest.NewRecorder()
		callbackRouter.ServeHTTP(response, callback)
		if response.Code != 204 {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":"owned-compose-accepted","object":"email","status":"accepted"}`))
	}))
	defer gateway.Close()
	certificateServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	certificate := certificateServer.TLS.Certificates[0]
	certificateServer.Close()
	backend := memory.New()
	user, _ := backend.Login(nil, "username", "password")
	box, _ := user.GetMailbox("INBOX")
	mailbox := box.(*memory.Mailbox)
	raw := []byte("From: Buyer <buyer@example.invalid>\r\nTo: windows-notify@example.invalid\r\nSubject: =?UTF-8?B?SU1BUCDmlLbku7bmtYvor5U=?=\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=fixture\r\n\r\n--fixture\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nIsolated inbound message\r\n--fixture\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n<p>Isolated inbound message</p><script>parent.__mailScriptExecuted=true</script><img src=\"https://tracker.example.invalid/pixel\">\r\n--fixture--\r\n")
	large := []byte("From: buyer@example.invalid\r\nTo: windows-notify@example.invalid\r\nSubject: Large archive fixture\r\nContent-Type: text/plain\r\n\r\n" + strings.Repeat("x", (1<<20)+128))
	raw = bytes.Replace(raw, []byte("--fixture--\r\n"), []byte("--fixture\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=fixture.txt\r\nContent-Transfer-Encoding: base64\r\n\r\nQXR0YWNobWVudCBmaXh0dXJl\r\n--fixture--\r\n"), 1)
	mailbox.Messages = []*memory.Message{{Uid: 6, Date: time.Now(), Body: raw, Size: uint32(len(raw))}, {Uid: 7, Date: time.Now(), Body: large, Size: uint32(len(large))}}
	server := imapserver.New(backend)
	server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	listener = tls.NewListener(listener, server.TLSConfig)
	go server.Serve(listener)
	defer server.Close()
	host, rawPort, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(rawPort)
	pin := sha256.Sum256(certificate.Certificate[0])
	command := notificationmail.Command{OperationID: "00000000-0000-4000-8000-000000000009", ExpectedRevision: directory.Revision, ID: account.ID, Name: account.Name, Email: account.Email, ReplyTo: account.ReplyTo, Enabled: true, Registration: true, Transport: &notificationmail.TransportConfig{Provider: "quguo", Relayme: &maildelivery.RelaymeConfig{APIURL: gateway.URL, KeyID: "owned", Secret: "fixture-secret"}, IMAP: &maildelivery.IMAPConfig{Host: host, Port: port, Username: "username", Password: "password", Encryption: "tls", CertSHA256: hex.EncodeToString(pin[:])}}}
	if _, e = store.Save(t.Context(), actor, command); e != nil {
		t.Fatal("owned IMAP setup failed")
	}
	ready, _ := json.Marshal(map[string]any{"account_id": account.ID, "ready": true})
	if os.WriteFile(filepath.Join(logs, "inbox-ready.json"), ready, 0600) != nil {
		t.Fatal("ready evidence failed")
	}
	for index := 0; index < 1200; index++ {
		if _, e = os.Stat(filepath.Join(logs, "inbox-browser-done")); e == nil {
			break
		}
		if index == 1199 {
			t.Fatal("inbox browser did not finish")
		}
		time.Sleep(100 * time.Millisecond)
	}
	var rows []struct {
		ID                string
		UID               uint32
		ReadAt            *time.Time
		Archived, Deleted bool
	}
	if db.Table("notification_mail_inbox").Where("account_id=?", account.ID).Order("uid").Find(&rows).Error != nil || len(rows) != 2 {
		t.Fatal("inbox was not uniquely imported")
	}
	for index, row := range rows {
		_, restored, e := store.MessageRaw(t.Context(), actor, row.ID)
		expected := raw
		if index == 1 {
			expected = large
		}
		if e != nil || !bytes.Equal(restored, expected) {
			t.Fatal("encrypted RFC822 archive differs")
		}
		clear(restored)
	}
	var parts int64
	db.Table("notification_mail_inbox_parts").Count(&parts)
	if parts != 3 {
		t.Fatal("large archive not chunked")
	}
	if rows[0].ReadAt == nil || rows[0].Archived || rows[0].Deleted {
		t.Fatal("read/archive/trash restore terminal state differs")
	}
	for _, message := range mailbox.Messages {
		if len(message.Flags) != 0 {
			t.Fatal("remote Seen flag mutated")
		}
	}
	var cursor struct{ UIDValidity, ScannedUID uint32 }
	db.Table("notification_mail_sync").Where("account_id=?", account.ID).Take(&cursor)
	if cursor.UIDValidity != 1 || cursor.ScannedUID != 7 {
		t.Fatal("sync checkpoint differs")
	}
	// Fail after inserting the message header. Both its encrypted parts and the
	// cursor must roll back; the next explicit scan imports it exactly once.
	mailbox.Messages = append(mailbox.Messages, &memory.Message{Uid: 8, Date: time.Now(), Body: raw, Size: uint32(len(raw))})
	if db.Callback().Raw().Before("gorm:raw").Register("owned_inbox_archive_failure", func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "INSERT INTO notification_mail_inbox_parts") {
			tx.AddError(errors.New("owned inbox archive write fault"))
		}
	}) != nil {
		t.Fatal("archive fault setup failed")
	}
	_, fault := store.SyncInbox(t.Context(), actor, account.ID)
	if db.Callback().Raw().Remove("owned_inbox_archive_failure") != nil {
		t.Fatal("archive fault cleanup failed")
	}
	if fault == nil {
		t.Fatal("late archive write failure hidden")
	}
	var count int64
	db.Table("notification_mail_inbox").Where("account_id=?", account.ID).Count(&count)
	db.Table("notification_mail_sync").Where("account_id=?", account.ID).Take(&cursor)
	if count != 2 || cursor.ScannedUID != 7 {
		t.Fatal("failed archive advanced cursor or retained partial message")
	}
	recovered, e := store.SyncInbox(t.Context(), actor, account.ID)
	if e != nil || recovered.Imported != 1 || recovered.ScannedUID != 8 {
		t.Fatal("failed archive cannot recover")
	}
	replayed, e := store.SyncInbox(t.Context(), actor, account.ID)
	if e != nil || replayed.Imported != 0 {
		t.Fatal("archive replay duplicated a message")
	}
	if os.WriteFile(filepath.Join(logs, "inbox-sql-passed.json"), []byte(`{"browser_messages":2,"browser_encrypted_chunks":3,"raw_bytes_identical":true,"remote_seen_unchanged":true,"read_archive_trash_restore":true,"late_archive_failure_atomic":true,"recovery_imported_once":true,"final_checkpoint_uid":8}`), 0600) != nil {
		t.Fatal("inbox evidence failed")
	}
	if submissions.Load() != 1 {
		t.Fatal("manual send was not accepted exactly once")
	}
	var composed []struct {
		ID, Status      string
		ContentEnvelope []byte
	}
	if db.Table("notification_mail_compositions").Find(&composed).Error != nil || len(composed) != 1 || composed[0].Status != "sent" || bytes.Contains(composed[0].ContentEnvelope, []byte("Attachment fixture")) {
		t.Fatal("manual archive terminal state differs")
	}
	_, content, e := store.Composition(t.Context(), actor, composed[0].ID)
	if e != nil || len(content.Attachments) != 1 || content.Subject != "Compose fixture" {
		t.Fatal("encrypted draft did not round trip")
	}
	var earlyObserved struct{ DeliveryStatus *string }
	db.Table("notification_mail_compositions").Where("id=?", composed[0].ID).Take(&earlyObserved)
	if earlyObserved.DeliveryStatus == nil || *earlyObserved.DeliveryStatus != "delayed" {
		t.Fatal("callback before submission response was lost")
	}
	router := gin.New()
	_ = db.Callback().Create().After("gorm:create").Register("owned_callback_diagnostic", func(tx *gorm.DB) {
		var sqlError *driver.MySQLError
		if errors.As(tx.Error, &sqlError) {
			t.Logf("callback SQL error code=%d SQL=%s", sqlError.Number, tx.Statement.SQL.String())
		}
	})
	store.RegisterWebhooks(router)
	callbackTime := time.Now().UTC().Truncate(time.Second)
	postCallback := func(provider, eventID, body string, when time.Time, tamper bool) int {
		path := "/api/webhooks/mail/" + account.ID + "/" + provider
		request := httptest.NewRequest("POST", path, strings.NewReader(body))
		stamp := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte("fixture-secret"))
		if provider == "relayme" {
			digest := sha256.Sum256([]byte(body))
			_, _ = mac.Write([]byte("POST\n" + path + "\n" + stamp + "\nnonce\n" + hex.EncodeToString(digest[:])))
			request.Header.Set("X-Relayme-Key-ID", "owned")
			request.Header.Set("X-Relayme-Timestamp", stamp)
			request.Header.Set("X-Relayme-Nonce", "nonce")
			request.Header.Set("X-Relayme-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
		} else {
			_, _ = mac.Write([]byte(eventID + "." + stamp + "." + body))
			request.Header.Set("svix-id", eventID)
			request.Header.Set("svix-timestamp", stamp)
			request.Header.Set("svix-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		}
		if tamper {
			request.Body = io.NopCloser(strings.NewReader(body + " "))
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response.Code
	}
	relayEvent := func(id, status string, at time.Time) string {
		raw, _ := json.Marshal(map[string]any{"event_id": id, "event_type": "delivery_status", "occurred_at": at, "submission_id": "owned-compose-accepted", "mail_from": account.Email, "recipient": "compose@example.invalid", "status": status})
		return string(raw)
	}
	delivered := relayEvent("delivered", "success", callbackTime)
	first, second, third := postCallback("relayme", "", delivered, callbackTime, true), postCallback("relayme", "", delivered, callbackTime, false), postCallback("relayme", "", delivered, callbackTime, false)
	if first != 401 || second != 204 || third != 204 {
		t.Fatalf("Relayme signature or duplicate callback differs: %d/%d/%d", first, second, third)
	}
	if postCallback("relayme", "", relayEvent("delivered", "failed", callbackTime), callbackTime, false) != 409 {
		t.Fatal("conflicting callback ID accepted")
	}
	if postCallback("relayme", "", relayEvent("old-delayed", "delayed", callbackTime.Add(-time.Minute)), callbackTime, false) != 204 {
		t.Fatal("older status rejected")
	}
	var observed struct{ DeliveryStatus *string }
	db.Table("notification_mail_compositions").Where("id=?", composed[0].ID).Take(&observed)
	if observed.DeliveryStatus == nil || *observed.DeliveryStatus != "success" {
		t.Fatal("out-of-order callback overwrote latest delivery")
	}
	callbackRaw := bytes.Replace(raw, []byte("=?UTF-8?B?SU1BUCDmlLbku7bmtYvor5U=?="), []byte("Webhook triggered inbound"), 1)
	mailbox.Messages = append(mailbox.Messages, &memory.Message{Uid: 9, Date: callbackTime, Body: callbackRaw, Size: uint32(len(callbackRaw))})
	inbound, _ := json.Marshal(map[string]any{"event_id": "inbound-fixture", "occurred_at": callbackTime, "mailbox": "INBOX", "to_address": account.Email, "from_address": "buyer@example.invalid", "subject": "Webhook triggered inbound"})
	if postCallback("relayme", "", string(inbound), callbackTime, false) != 204 {
		t.Fatal("signed inbound did not resolve archived IMAP message")
	}
	db.Table("notification_mail_inbox").Where("account_id=? AND uid=9", account.ID).Count(&count)
	if count != 1 {
		t.Fatal("callback did not archive new raw IMAP message")
	}
	directory, e = store.Directory(t.Context(), actor)
	if e != nil {
		t.Fatal(e)
	}
	command.OperationID = "00000000-0000-4000-8000-000000000010"
	command.ExpectedRevision = directory.Revision
	command.Transport = &notificationmail.TransportConfig{Provider: "resend"}
	command.WebhookSecret = "whsec_" + base64.StdEncoding.EncodeToString([]byte("fixture-secret"))
	if _, e = store.Save(t.Context(), actor, command); e != nil {
		t.Fatal("restore Resend after compose failed")
	}
	resend, _ := json.Marshal(map[string]any{"type": "email.delivered", "created_at": callbackTime, "data": map[string]any{"email_id": "owned-compose-accepted", "to": []string{"compose@example.invalid"}}})
	if postCallback("resend", "resend-observed", string(resend), callbackTime, true) != 401 || postCallback("resend", "resend-observed", string(resend), callbackTime, false) != 204 {
		t.Fatal("Resend signed callback differs")
	}
	var events int64
	db.Table("notification_mail_callbacks").Count(&events)
	if events != 5 {
		t.Fatal("callback deduplication differs")
	}
	if os.WriteFile(filepath.Join(logs, "mail-callbacks-passed.json"), []byte(`{"signed_relayme":true,"signed_resend":true,"tampered_body_rejected":true,"duplicate_deduplicated":true,"conflicting_event_rejected":true,"older_status_ignored":true,"early_callback_reconciled":true,"inbound_callback_archived_uid":9,"stored_events":5,"external_sends":0}`), 0600) != nil {
		t.Fatal("callback evidence failed")
	}
}
