package identity_test

import (
	"context"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/notificationmail"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"sync"
	"testing"
)

func checkOrderMailJobs(t *testing.T, ctx context.Context, db *gorm.DB, user identity.User, store notificationmail.Store, call func(string, string, string, string, string, int) []byte, csrf string) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("order mail admin fixture")
		}
	}
	const path = "/api/admin/notification-mail/order-jobs"
	const origin = "https://admin.example"
	id := uuid.NewString()
	exec("INSERT INTO orders(id,version,status,is_draft_order,currency_code,created_at,updated_at) VALUES('mail-admin-order',1,'pending',FALSE,'eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO order_notification_outbox(id,order_id,event,payload_envelope,status,attempts,created_at) VALUES(?,'mail-admin-order','order.placed','SECRET_RECIPIENT_PAYLOAD','failed',3,UTC_TIMESTAMP(6))", id)
	exec("INSERT INTO order_mail_deliveries(notification_id,first_attempt_at,error_code,request_envelope,sender_fingerprint,failed_at) VALUES(?,UTC_TIMESTAMP(6),'network_error','SECRET_BODY',REPEAT('a',64),UTC_TIMESTAMP(6))", id)
	raw := call("GET", path+"?offset=0", "", origin, "", 200)
	var list notificationmail.Jobs
	if json.Unmarshal(raw, &list) != nil || len(list.Jobs) != 1 || !list.Jobs[0].Retryable || list.Jobs[0].Attempts != 3 {
		t.Fatal("mail job projection differs")
	}
	for _, secret := range []string{"SECRET", "request_envelope", "payload_envelope", "sender_fingerprint", "provider_id"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("mail job leaked content")
		}
	}
	call("GET", path+"?offset=01", "", origin, "", 400)
	call("GET", path+"?fields=*", "", origin, "", 400)
	in := notificationmail.RetryOrderJob{OperationID: uuid.NewString(), ExpectedAttempts: 3}
	body, _ := json.Marshal(in)
	retry := path + "/" + id + "/retry"
	call("POST", retry, string(body), origin, "", 403)
	call("POST", retry, string(body), "https://foreign.example", csrf, 403)
	call("POST", retry, `{"operation_id":"x","operation_id":"y","expected_attempts":3}`, origin, csrf, 400)
	call("POST", retry, `{"operation_id":"x","expected_attempts":null}`, origin, csrf, 400)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := store.RetryOrderJob(ctx, user, id, in); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	// Acknowledged replays must not reset a later worker claim or sent result.
	exec("UPDATE order_notification_outbox SET status='sent',attempts=1 WHERE id=?", id)
	var replay notificationmail.RetryJobResult
	if json.Unmarshal(call("POST", retry, string(body), origin, csrf, 200), &replay) != nil || !replay.Replayed {
		t.Fatal("retry receipt missing")
	}
	var status string
	if db.Table("order_notification_outbox").Select("status").Where("id=?", id).Scan(&status).Error != nil || status != "sent" {
		t.Fatal("replay reset sent mail")
	}
	changed := in
	changed.ExpectedAttempts = 1
	if _, e := store.RetryOrderJob(ctx, user, id, changed); e != notificationmail.ErrConflict {
		t.Fatal("changed command replay accepted")
	}
	var n int64
	if db.Table("audit_events").Where("action='order_mail.retry' AND resource_id=?", id).Count(&n).Error != nil || n != 1 {
		t.Fatal("retry audit duplicated")
	}
	exec("UPDATE order_notification_outbox SET status='failed',attempts=3 WHERE id=?", id)
	exec("UPDATE order_mail_deliveries SET first_attempt_at=UTC_TIMESTAMP(6)-INTERVAL 24 HOUR WHERE notification_id=?", id)
	changed.OperationID = uuid.NewString()
	changed.ExpectedAttempts = 3
	if _, e := store.RetryOrderJob(ctx, user, id, changed); e != notificationmail.ErrConflict {
		t.Fatal("expired retry accepted")
	}
	exec("UPDATE order_mail_deliveries SET first_attempt_at=UTC_TIMESTAMP(6),lease_until=UTC_TIMESTAMP(6)+INTERVAL 1 MINUTE WHERE notification_id=?", id)
	if _, e := store.RetryOrderJob(ctx, user, id, changed); e != notificationmail.ErrConflict {
		t.Fatal("live lease retried")
	}
	exec("UPDATE order_mail_deliveries SET lease_until=NULL WHERE notification_id=?", id)
	exec("CREATE TRIGGER fail_order_mail_retry BEFORE INSERT ON audit_events FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned retry failure'")
	_, e := store.RetryOrderJob(ctx, user, id, changed)
	exec("DROP TRIGGER fail_order_mail_retry")
	if e == nil {
		t.Fatal("late audit failure accepted")
	}
	if db.Table("order_notification_outbox").Select("status").Where("id=?", id).Scan(&status).Error != nil || status != "failed" {
		t.Fatal("failed retry partially committed")
	}
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='mail.write'", user.ID)
	call("POST", retry, string(body), origin, csrf, 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) SELECT role_id,'mail.write' FROM authorization_memberships WHERE user_id=?", user.ID)
	t.Log(`ORDER_MAIL_ADMIN_REHEARSAL {"real_mysql_http":true,"private_projection":true,"auth_origin_csrf":true,"concurrent_retry_single_audit":true,"sent_replay_unchanged":true,"expired_and_leased_denied":true,"late_failure_atomic":true,"revoked_permission_denies_replay":true}`)
}
