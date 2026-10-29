package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/maildelivery"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/notificationmail"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkNotificationMail(t *testing.T, ctx context.Context, db *gorm.DB, user identity.User, router *gin.Engine, sessions *sessionstore.Store, call func(string, string, string, string, string, int) []byte, csrf string) {
	t.Helper()
	ring, _ := protected.New("fixture", map[string][]byte{"fixture": bytes.Repeat([]byte{82}, 32)})
	store := notificationmail.Store{DB: db, Ring: ring}
	auth, e := identity.NewWithOptions(db, identity.Options{Sessions: sessions, Origin: "https://admin.example", Secure: true})
	if e != nil {
		t.Fatal("mail auth fixture failed")
	}
	group := router.Group("/api/admin")
	group.Use(auth.Require)
	store.Register(group)
	exec := func(query string, args ...any) {
		t.Helper()
		if db.WithContext(ctx).Exec(query, args...).Error != nil {
			t.Fatal("mail fixture SQL failed")
		}
	}
	const path = "/api/admin/notification-mail/accounts"
	const origin = "https://admin.example"
	directory := func() notificationmail.Directory {
		t.Helper()
		var out notificationmail.Directory
		raw := call("GET", path, "", origin, "", 200)
		if json.Unmarshal(raw, &out) != nil || out.Revision == "" {
			t.Fatal("mail directory failed")
		}
		for _, secret := range []string{"re_artificial_mail_key", "artificial-webhook-secret", "request_payload", "\"api_key\"", "\"webhook_secret\""} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("mail response disclosed secret")
			}
		}
		return out
	}
	initial := directory()
	if len(initial.Accounts) != 0 || initial.RegistrationAccountID != nil {
		t.Fatal("historical/default mail sender adopted")
	}
	if _, e := store.RegistrationSender(ctx); e == nil {
		t.Fatal("unconfigured sender accepted")
	}
	in := notificationmail.Command{OperationID: uuid.NewString(), ExpectedRevision: initial.Revision, Name: "EuroRefurb 中文", Email: "notification@example.invalid", ReplyTo: "reply@example.invalid", APIKey: "re_artificial_mail_key", WebhookSecret: "artificial-webhook-secret", Enabled: true, Registration: true}
	encode := func(in notificationmail.Command) string { raw, _ := json.Marshal(in); return string(raw) }
	call("POST", path, encode(in), origin, "", 403)
	call("POST", path, encode(in), "https://foreign.example", csrf, 403)
	call("POST", path+"?fields=*", encode(in), origin, csrf, 400)
	call("POST", path, `{"enabled":true,"enabled":false}`, origin, csrf, 400)
	call("POST", path, strings.Repeat(" ", 16385)+"{}", origin, csrf, 413)
	var result notificationmail.Result
	if json.Unmarshal(call("POST", path, encode(in), origin, csrf, 200), &result) != nil || result.ID == "" || result.Replayed {
		t.Fatal("new mailbox HTTP save failed")
	}
	replay, e := store.Save(ctx, user, in)
	if e != nil || !replay.Replayed || replay.ID != result.ID {
		t.Fatal("mail creation replay changed identity")
	}
	changed := in
	changed.APIKey = "re_other_artificial_key"
	if _, e := store.Save(ctx, user, changed); !errors.Is(e, notificationmail.ErrConflict) {
		t.Fatal("mail operation accepted different secret")
	}
	after := directory()
	if len(after.Accounts) != 1 || after.RegistrationAccountID == nil || *after.RegistrationAccountID != result.ID || !after.Accounts[0].HasAPIKey || !after.Accounts[0].HasWebhookSecret {
		t.Fatal("mail directory projection incomplete")
	}
	var encrypted struct{ APIKey, WebhookSecret []byte }
	if db.Table("mail_accounts").Select("api_key,webhook_secret").Where("id=?", result.ID).Take(&encrypted).Error != nil || bytes.Contains(encrypted.APIKey, []byte(in.APIKey)) || bytes.Contains(encrypted.WebhookSecret, []byte(in.WebhookSecret)) {
		t.Fatal("mail credentials not encrypted")
	}
	var receipt struct{ RequestPayload []byte }
	if db.Raw("SELECT request_payload FROM notification_mail_commands WHERE operation_id=?", in.OperationID).Scan(&receipt).Error != nil || bytes.Contains(receipt.RequestPayload, []byte(in.APIKey)) {
		t.Fatal("mail receipt not encrypted")
	}
	var audit string
	if db.Raw("SELECT data FROM audit_events WHERE action='notification_mail.save' AND resource_id=?", result.ID).Scan(&audit).Error != nil || strings.Contains(audit, in.APIKey) || strings.Contains(audit, in.WebhookSecret) {
		t.Fatal("mail audit leaked credentials")
	}
	sender, e := store.RegistrationSender(ctx)
	if e != nil {
		t.Fatal("saved mailbox could not create registration sender")
	}
	orderSender, orderErr := store.OrderSender(ctx)
	if orderErr != nil || orderSender == nil {
		t.Fatal("approved order sender missing")
	}
	orderBody, orderErr := orderSender.PrepareOrder(maildelivery.OrderPlaced{To: "new@example.invalid", Reference: "2026103000000000001", Amount: "158.75", Currency: "eur", Locale: "zh"})
	if orderErr != nil || !bytes.Contains(orderBody, []byte("notification@example.invalid")) {
		t.Fatal("approved order sender content differs")
	}
	prepared, e := sender.Prepare("new@example.invalid", "https://shop.example/dk-en/verify-account?token=artificial", "zh")
	if e != nil || !bytes.Contains(prepared, []byte("reply@example.invalid")) {
		t.Fatal("sender mailbox reply address missing")
	}
	var payload map[string]any
	if json.Unmarshal(prepared, &payload) != nil || !strings.Contains(fmt.Sprint(payload["from"]), "notification@example.invalid") {
		t.Fatal("sender mailbox from address missing")
	}
	customers, err := customer.New(db, ring, sessions, "https://shop.example")
	if err != nil {
		t.Fatal("mailbox registration store unavailable")
	}
	account, err := customers.Register(ctx, customer.Registration{Email: "mailbox-registration@example.invalid", Password: "fixture", FirstName: "New", LastName: "Customer", Country: "dk", Locale: "en"})
	if err != nil {
		t.Fatal("mailbox registration queue failed")
	}
	verifiedToken := ""
	oracle := &deliveryOracle{sender: sender, send: func(_ context.Context, _ string, body []byte) maildelivery.Result {
		var mail map[string]any
		if json.Unmarshal(body, &mail) != nil {
			t.Fatal("mailbox verification body invalid")
		}
		if !strings.Contains(fmt.Sprint(mail["from"]), in.Email) || fmt.Sprint(mail["reply_to"]) != in.ReplyTo {
			t.Fatal("verification ignored configured mailbox")
		}
		lines := strings.Split(fmt.Sprint(mail["text"]), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "https://shop.example/") {
				parsed, e := url.Parse(line)
				if e == nil {
					verifiedToken = parsed.Query().Get("token")
				}
			}
		}
		return maildelivery.Result{ID: "00000000-0000-4000-8000-000000000091"}
	}}
	delivered, err := customers.DispatchVerification(ctx, oracle)
	if err != nil || delivered.Status != "sent" || verifiedToken == "" || customers.Verify(ctx, verifiedToken) != nil {
		t.Fatal("configured mailbox to registration verification chain failed")
	}
	if _, err := customers.Login(ctx, account.Email, "fixture"); err != nil {
		t.Fatal("configured mailbox verified account cannot sign in")
	}
	edit := in
	edit.ID = result.ID
	edit.OperationID = uuid.NewString()
	edit.ExpectedRevision = after.Revision
	edit.APIKey = ""
	edit.WebhookSecret = ""
	edit.Name = "Renamed"
	edit.Registration = false
	if _, e := store.Save(ctx, user, edit); e != nil {
		t.Fatal("blank secret preservation failed")
	}
	if fallback, e := store.RegistrationSender(ctx); e != nil || fallback == nil {
		t.Fatal("original oldest-enabled sender fallback missing")
	}
	var current struct{ APIKey, WebhookSecret []byte }
	if db.Table("mail_accounts").Select("api_key,webhook_secret").Where("id=?", result.ID).Take(&current).Error != nil {
		t.Fatal("saved secret read failed")
	}
	for column, envelope := range map[string][]byte{"api_key": current.APIKey, "webhook_secret": current.WebhookSecret} {
		plain, e := ring.Open(protected.Context{Purpose: "merchant-config", Table: "mail_accounts", Column: column, Row: result.ID}, envelope)
		expected := in.APIKey
		if column == "webhook_secret" {
			expected = in.WebhookSecret
		}
		if e != nil || string(plain) != expected {
			t.Fatal("blank credential was replaced")
		}
		clear(plain)
		if _, e := ring.Open(protected.Context{Purpose: "merchant-config", Table: "mail_accounts", Column: column, Row: "other"}, envelope); e == nil {
			t.Fatal("mail envelope row binding missing")
		}
	}
	// Current revision checks and full-value unique claims prevent lost updates.
	stale := edit
	stale.OperationID = uuid.NewString()
	if _, e := store.Save(ctx, user, stale); !errors.Is(e, notificationmail.ErrConflict) {
		t.Fatal("stale mailbox overwrite accepted")
	}
	d := directory()
	duplicate := in
	duplicate.OperationID = uuid.NewString()
	duplicate.ExpectedRevision = d.Revision
	if _, e := store.Save(ctx, user, duplicate); !errors.Is(e, notificationmail.ErrConflict) {
		t.Fatal("duplicate email claim accepted")
	}
	concurrent := edit
	concurrent.OperationID = uuid.NewString()
	concurrent.ExpectedRevision = d.Revision
	concurrent.Registration = true
	var wg sync.WaitGroup
	results := make(chan notificationmail.Result, 8)
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); r, e := store.Save(ctx, user, concurrent); results <- r; errs <- e }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("concurrent mail replay failed")
		}
	}
	first := 0
	for r := range results {
		if !r.Replayed {
			first++
		}
	}
	if first != 1 {
		t.Fatal("mail save executed multiple times")
	}
	// Late audit failure rolls back the mailbox, default selection, claim and receipt.
	d = directory()
	late := in
	late.OperationID = uuid.NewString()
	late.ExpectedRevision = d.Revision
	late.Email = "rollback@example.invalid"
	exec("CREATE TRIGGER reject_mail_audit BEFORE INSERT ON audit_events FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='fixture'")
	if _, e := store.Save(ctx, user, late); e == nil {
		t.Fatal("late mail write failure hidden")
	}
	exec("DROP TRIGGER reject_mail_audit")
	if directory().Revision != d.Revision {
		t.Fatal("late mailbox failure changed revision")
	}
	if _, e := store.Save(ctx, user, late); e != nil {
		t.Fatal("rolled back claim or operation blocked retry")
	}
	// Permission is checked again before replay; a revoked manager cannot recover secrets/results.
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='mail.write'", user.ID)
	call("POST", path, encode(in), origin, csrf, 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) SELECT role_id,'mail.write' FROM authorization_memberships WHERE user_id=?", user.ID)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", user.ID)
	if _, e := store.Directory(ctx, user); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("warehouse read full mail settings")
	}
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", user.ID)
	// An imported account with a disposition cannot be activated even after a bad manual approval.
	d = directory()
	held := edit
	held.OperationID = uuid.NewString()
	held.ExpectedRevision = d.Revision
	held.Registration = true
	exec("INSERT INTO migration_merchant_dispositions(source_table,source_id,snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version,disposition,reason) VALUES('mail_account',?,REPEAT('a',32),REPEAT('b',32),REPEAT('c',32),REPEAT('d',16),1,'hold','legacy-merchant-reconciliation')", result.ID)
	if _, e := store.Save(ctx, user, held); !errors.Is(e, notificationmail.ErrHeld) {
		t.Fatal("held imported mailbox was edited")
	}
	exec("UPDATE notification_mail_settings SET registration_account_id=? WHERE id=1", result.ID)
	if _, e := store.RegistrationSender(ctx); !errors.Is(e, notificationmail.ErrHeld) {
		t.Fatal("held imported mailbox sent mail")
	}
	exec("UPDATE notification_mail_settings SET registration_account_id=NULL WHERE id=1")
	t.Log(`NOTIFICATION_MAIL_REHEARSAL {"real_mysql_http":true,"admin_session_csrf":true,"separate_grants":true,"warehouse_denied":true,"credentials_encrypted":true,"secrets_not_returned":true,"blank_preserves_secrets":true,"exact_operation_replay":true,"concurrent_replays":8,"single_execution":true,"stale_revision_denied":true,"duplicate_address_denied":true,"late_audit_failure_atomic":true,"revoked_permission_denies_replay":true,"historical_hold_denied":true,"database_selected_sender":true,"configured_mailbox_to_queue_to_synthetic_sender_to_verify":true,"actual_external_email":false}`)
}
