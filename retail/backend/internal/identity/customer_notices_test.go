package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/customernotices"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkCustomerNotices(t *testing.T, ctx context.Context, db *gorm.DB, store *customer.Store, token, owner string) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("notice fixture", e)
		}
	}
	for i := 0; i < 23; i++ {
		exec("INSERT INTO customer_account_notices(id,customer_id,event_key,kind,resource_id,reference,action,created_at) VALUES(?,?,?,'shipment.created','notice-order','REF-001','',?)", "notice_"+uuid.NewString(), owner, fmt.Sprintf("fixture-shipment:%d", i), time.Date(2026, 10, 1, 0, 0, i, 0, time.UTC))
	}
	p, e := store.Notices(ctx, token, 0)
	if e != nil || p.Count != 23 || p.Unread != 23 || len(p.Notifications) != 20 || p.Offset != 0 || p.Limit != 20 {
		t.Fatal("notice first page", e, p)
	}
	last, e := store.Notices(ctx, token, 20)
	if e != nil || len(last.Notifications) != 3 || last.Unread != 23 {
		t.Fatal("notice last page", e, last)
	}
	for i := 1; i < len(p.Notifications); i++ {
		if !p.Notifications[i-1].CreatedAt.After(p.Notifications[i].CreatedAt) {
			t.Fatal("notice sort")
		}
	}
	if p.Notifications[19].CreatedAt.Before(last.Notifications[0].CreatedAt) {
		t.Fatal("notice page overlap")
	}
	raw, _ := json.Marshal(p)
	for _, secret := range []string{"customer_id", "event_key", "password", "token"} {
		if containsJSONKey(raw, secret) {
			t.Fatal("notice private projection", secret)
		}
	}
	var wg sync.WaitGroup
	results := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- store.ReadNotices(ctx, token, p.Notifications[0].ID) }()
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal("concurrent read", e)
		}
	}
	p, e = store.Notices(ctx, token, 0)
	if e != nil || p.Unread != 22 || p.Notifications[0].ReadAt == nil {
		t.Fatal("single read", e, p.Unread)
	}
	if e := store.ReadNotices(ctx, token, "notice_"+uuid.NewString()); e != nil {
		t.Fatal("unknown notice no-op", e)
	}
	if e := store.ReadNotices(ctx, token, ""); e != nil {
		t.Fatal("mark all read", e)
	}
	p, e = store.Notices(ctx, token, 0)
	if e != nil || p.Unread != 0 || p.Count != 23 {
		t.Fatal("all read", e, p)
	}
	for _, offset := range []int{-1, 100001} {
		if _, e := store.Notices(ctx, token, offset); !errors.Is(e, customer.ErrInvalid) {
			t.Fatal("offset accepted")
		}
	}
	if e := store.ReadNotices(ctx, token, "bad"); !errors.Is(e, customer.ErrInvalid) {
		t.Fatal("notice id accepted")
	}
	if _, e := store.Notices(ctx, "bad", 0); !errors.Is(e, customer.ErrUnauthorized) {
		t.Fatal("anonymous notice accepted")
	}
	// The emitter must refuse autocommit and unregistered kinds.
	if customernotices.EmitOrderTx(db, "notice-order", "shipment.created", "notice-fulfillment") == nil {
		t.Fatal("autocommit notice emitter")
	}
	other := "cust_notice_other"
	exec("INSERT INTO customer_accounts(id,email,password_hash,first_name,last_name,enabled,auth_version,verified_at,created_at,updated_at) VALUES(?,'notice-other@example.invalid','artificial','Other','Customer',TRUE,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", other)
	foreign := "notice_" + uuid.NewString()
	exec("INSERT INTO customer_account_notices(id,customer_id,event_key,kind,resource_id,reference,created_at) VALUES(?,?,'foreign-event','shipment.created','foreign-order','FOREIGN',UTC_TIMESTAMP(6))", foreign, other)
	if e := store.ReadNotices(ctx, token, foreign); e != nil {
		t.Fatal("foreign read no-op", e)
	}
	var foreignUnread int64
	if db.Table("customer_account_notices").Where("id=? AND read_at IS NULL", foreign).Count(&foreignUnread).Error != nil || foreignUnread != 1 {
		t.Fatal("foreign notice changed")
	}
	exec("INSERT INTO orders(id,version,status,is_draft_order,currency_code,custom_display_id,created_at,updated_at) VALUES('notice-event-order',1,'pending',FALSE,'eur','NOTICE-REF',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	emit := func(resource string) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return customernotices.EmitOrderTx(tx, "notice-event-order", "shipment.created", resource)
		})
	}
	if e := emit("notice-unowned"); e != nil {
		t.Fatal("unowned event", e)
	}
	exec("INSERT INTO customer_order_ownership(order_id,customer_id,cart_id,store_id,created_at) VALUES('notice-event-order',?,'notice-fixture-cart','notice-fixture-store',UTC_TIMESTAMP(6))", owner)
	for range 2 {
		if e := emit("notice-owned"); e != nil {
			t.Fatal("owned event", e)
		}
	}
	p, e = store.Notices(ctx, token, 0)
	if e != nil || p.Count != 24 || p.Unread != 1 || p.Notifications[0].Reference != "NOTICE-REF" {
		t.Fatal("event owner and dedupe", e, p)
	}
	rollback := errors.New("artificial owner rollback")
	e = db.Transaction(func(tx *gorm.DB) error {
		if e := customernotices.EmitOrderTx(tx, "notice-event-order", "after_sales.processing", "notice-asr"); e != nil {
			return e
		}
		return rollback
	})
	if !errors.Is(e, rollback) {
		t.Fatal("notice rollback", e)
	}
	var leaked int64
	if db.Table("customer_account_notices").Where("event_key='after_sales.processing:notice-asr'").Count(&leaked).Error != nil || leaked != 0 {
		t.Fatal("notice escaped rollback")
	}
	exec("CREATE TRIGGER notice_insert_failure BEFORE INSERT ON customer_account_notices FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial notice failure'")
	e = db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("UPDATE orders SET custom_display_id='MUST-ROLLBACK' WHERE id='notice-event-order'").Error; e != nil {
			return e
		}
		return customernotices.EmitOrderTx(tx, "notice-event-order", "after_sales.completed", "notice-asr")
	})
	exec("DROP TRIGGER notice_insert_failure")
	var reference string
	if e == nil || db.Table("orders").Select("custom_display_id").Where("id='notice-event-order'").Scan(&reference).Error != nil || reference != "NOTICE-REF" {
		t.Fatal("notice late failure not atomic", e, reference)
	}
	exec("UPDATE customer_accounts SET enabled=FALSE WHERE id=?", owner)
	if _, e := store.Notices(ctx, token, 0); !errors.Is(e, customer.ErrUnauthorized) {
		t.Fatal("disabled notice read")
	}
	exec("UPDATE customer_accounts SET enabled=TRUE WHERE id=?", owner)
	t.Log(`CUSTOMER_NOTICES_REHEARSAL {"new_account_only":true,"private_projection":true,"pagination_20":true,"global_unread":true,"single_read_concurrent":4,"mark_all_read":true,"unknown_id_noop":true,"invalid_offset_id_denied":true,"anonymous_disabled_denied":true,"autocommit_emitter_denied":true}`)
}

func containsJSONKey(raw []byte, key string) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	if _, ok := value[key]; ok {
		return true
	}
	var rows []map[string]json.RawMessage
	if json.Unmarshal(value["notifications"], &rows) == nil {
		for _, r := range rows {
			if _, ok := r[key]; ok {
				return true
			}
		}
	}
	return false
}
