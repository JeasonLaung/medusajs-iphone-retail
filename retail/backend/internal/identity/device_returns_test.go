package identity_test

import (
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"sync"
	"testing"
)

func checkDeviceReturns(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	ctx := t.Context()
	h := devices.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("device return fixture", e)
		}
	}
	exec("UPDATE device_runtime_schema SET identity_enabled=TRUE,commands_enabled=TRUE WHERE id=1")
	const path = "/api/admin/orders/allocation-order/after-sales/asr-receive/device-returns"
	var aid string
	db.Table("device_assignments").Select("id").Where("device_id='allocation-b' AND status='shipped' AND deleted_at IS NULL").Scan(&aid)
	if aid == "" {
		t.Fatal("shipped fixture absent")
	}
	exec(`INSERT INTO after_sales_requests(id,order_id,customer_id,type,reason,items,status,created_at,updated_at) VALUES('asr-receive','allocation-order','new-fixture-customer','return','Fixture return','{"lines":[{"item_id":"allocation-line","quantity":1,"device_ids":["allocation-b"]}]}','processing',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	defer exec("UPDATE device_return_runtime SET enabled=FALSE WHERE id=1")
	read := func() devices.ReturnContext {
		t.Helper()
		raw := call("GET", path, "", 200)
		if strings.Contains(string(raw), "cost_cents") || strings.Contains(string(raw), "imei") || strings.Contains(string(raw), "notes") {
			t.Fatal("private device facts exposed")
		}
		var r devices.ReturnContext
		if json.Unmarshal(raw, &r) != nil {
			t.Fatal("device return context")
		}
		return r
	}
	body := func(in devices.ReturnInput) string { b, _ := json.Marshal(in); return string(b) }
	before := read()
	if before.Enabled || len(before.Devices) != 1 {
		t.Fatal("default gate or device selection")
	}
	source := "allocation-source"
	for _, s := range before.Sources {
		if s.LocationID == "allocation-location" {
			source = s.ID
			break
		}
	}
	in := devices.ReturnInput{OperationID: uuid.NewString(), ExpectedRevision: before.Revision, AssignmentID: aid, SourceID: source, Scan: before.Devices[0].SerialNumber}
	call("POST", path, body(in), 503)
	exec("UPDATE device_return_runtime SET enabled=TRUE WHERE id=1")
	in.ExpectedRevision = read().Revision
	for _, wire := range []string{`{"scan":null}`, `{"scan":"a","scan":"b"}`, body(in) + ` {}`} {
		call("POST", path, wire, 400)
	}
	call("GET", path+"?unknown=1", "", 400)
	call("GET", "/api/admin/orders/foreign-order/after-sales/asr-receive/device-returns", "", 404)
	wrong := in
	wrong.Scan = "WRONG-MACHINE"
	call("POST", path, body(wrong), 409)
	exec("UPDATE after_sales_requests SET status='pending' WHERE id='asr-receive'")
	wrong = in
	wrong.ExpectedRevision = read().Revision
	call("POST", path, body(wrong), 409)
	exec("UPDATE after_sales_requests SET status='processing' WHERE id='asr-receive'")
	stale := in
	exec("UPDATE after_sales_requests SET updated_at=DATE_ADD(updated_at,INTERVAL 1 SECOND) WHERE id='asr-receive'")
	call("POST", path, body(stale), 409)
	in.ExpectedRevision = read().Revision
	stock := func() string {
		var s string
		db.Raw("SELECT CONCAT(CAST(stocked_quantity AS CHAR),'/',CAST(reserved_quantity AS CHAR)) FROM inventory_levels WHERE id='packing-target'").Row().Scan(&s)
		return s
	}
	baseline := stock()
	exec("CREATE TRIGGER return_receipt_fault BEFORE INSERT ON device_return_receipts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial receipt fault'")
	call("POST", path, body(in), 503)
	exec("DROP TRIGGER return_receipt_fault")
	if read().Devices[0].Status != "shipped" || stock() != baseline {
		t.Fatal("return rollback")
	}
	var n int64
	if db.Table("returns").Where("order_id='allocation-order'").Count(&n).Error != nil || n != 0 {
		t.Fatal("return facts not rolled back")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := h.ReceiveReturn(ctx, actor, "allocation-order", "asr-receive", in)
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("concurrent receive", e)
		}
	}
	exec("UPDATE device_return_runtime SET enabled=FALSE WHERE id=1")
	call("POST", path, body(in), 200)
	exec("UPDATE device_return_runtime SET enabled=TRUE WHERE id=1")
	if stock() != baseline || read().Devices[0].Status != "returned" || read().Devices[0].State != "inspection" {
		t.Fatal("quarantine stock/status")
	}
	for _, table := range []string{"device_return_receipts", "returns"} {
		if db.Table(table).Where("order_id='allocation-order'").Count(&n).Error != nil || n != 1 {
			t.Fatal("duplicate receipt/native return")
		}
	}
	var quantities struct{ Requested, Dismissed, WrittenOff, Received string }
	db.Raw("SELECT CAST(return_requested_quantity AS CHAR) AS requested,CAST(return_dismissed_quantity AS CHAR) AS dismissed,CAST(written_off_quantity AS CHAR) AS written_off,CAST(return_received_quantity AS CHAR) AS received FROM order_items WHERE order_id='allocation-order' AND item_id='allocation-line' AND version=1").Scan(&quantities)
	if quantities.Requested != "0.000000000000000000" || quantities.Received != "0.000000000000000000" || quantities.Dismissed != "1.000000000000000000" || quantities.WrittenOff != "1.000000000000000000" {
		t.Fatal("quarantine counters", quantities)
	}
	wrong = in
	wrong.OperationID = uuid.NewString()
	wrong.ExpectedRevision = read().Revision
	call("POST", path, body(wrong), 409)
	var role string
	db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='device.intake'", role)
	if _, e := h.ReceiveReturn(ctx, actor, "allocation-order", "asr-receive", in); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("revoked replay")
	}
	call("POST", path, body(in), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'device.intake')", role)
	exec("UPDATE devices SET deleted_at=UTC_TIMESTAMP(6) WHERE id='allocation-b'")
	call("POST", path, body(in), 404)
	exec("UPDATE devices SET deleted_at=NULL WHERE id='allocation-b'")
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("GET", path, "", 403)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	// Original no-device-id history still enforces requested quantity across machines.
	exec("UPDATE devices SET state='shipped' WHERE id='allocation-a'")
	exec("UPDATE device_assignments SET status='shipped' WHERE device_id='allocation-a' AND status IN ('reserved','packed') AND deleted_at IS NULL")
	exec(`UPDATE after_sales_requests SET items='{"lines":[{"item_id":"allocation-line","quantity":1}]}' WHERE id='asr-receive'`)
	var secondID string
	db.Table("device_assignments").Select("id").Where("device_id='allocation-a' AND status='shipped' AND deleted_at IS NULL").Scan(&secondID)
	second := read()
	for _, v := range second.Devices {
		if v.AssignmentID == secondID {
			wrong = devices.ReturnInput{OperationID: uuid.NewString(), ExpectedRevision: second.Revision, AssignmentID: secondID, SourceID: source, Scan: v.SerialNumber}
		}
	}
	call("POST", path, body(wrong), 409)
	// A shipped repaired machine can enter a later request; its old receipt cannot replay across requests.
	exec("UPDATE after_sales_requests SET status='completed' WHERE id='asr-receive'")
	exec("UPDATE devices SET imei2='000000000000987' WHERE id='allocation-b'")
	for _, kind := range []string{"repair", "exchange"} {
		key := "asr-receive-" + kind
		exec("UPDATE devices SET state='shipped' WHERE id='allocation-b'")
		exec("UPDATE device_assignments SET status='shipped' WHERE id=?", aid)
		exec(`INSERT INTO after_sales_requests(id,order_id,customer_id,type,reason,items,status,created_at,updated_at) VALUES(?,'allocation-order','new-fixture-customer',?,'Fixture repeat receipt','{"lines":[{"item_id":"allocation-line","quantity":1,"device_ids":["allocation-b"]}]}','processing',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, key, kind)
		current, e := h.ReadReturn(ctx, actor, "allocation-order", key)
		if e != nil {
			t.Fatal("repeat read", e)
		}
		next := devices.ReturnInput{OperationID: uuid.NewString(), ExpectedRevision: current.Revision, AssignmentID: aid, SourceID: source, Scan: "000000000000987"}
		result, e := h.ReceiveReturn(ctx, actor, "allocation-order", key, next)
		if e != nil || result.ReturnID != "" || kind == "repair" && result.State != "repair" || kind == "exchange" && result.State != "inspection" {
			t.Fatal("repair/exchange receipt", e)
		}
		call("POST", path, body(in), 404)
		exec("UPDATE after_sales_requests SET status='completed' WHERE id=?", key)
	}
	if stock() != baseline {
		t.Fatal("repair/exchange altered quantity stock")
	}
	t.Log(`DEVICE_RETURN_REHEARSAL {"real_gin_mysql_redis":true,"wrong_device_rejected":true,"processing_required":true,"concurrent_attempts":4,"single_return_and_receipt":true,"late_failure_atomic":true,"no_quantity_restock":true,"non_restocking_return_counters":true,"revoked_and_deleted_replay_denied":true,"full_store_warehouse_denied":true,"quantity_cap":true,"stale_request_denied":true,"paused_write_replay":true,"repair_and_exchange_quarantine":true,"later_repaired_device_request":true,"cross_request_receipt_denied":true}`)
}
