package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/buyback"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func checkBuybackRead(t *testing.T, db *gorm.DB, customers *customer.Store, router *gin.Engine, token, store, id string) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("buyback read fixture")
		}
	}
	read := func(account, shop, request string) (buyback.RequestDetail, error) {
		var out buyback.RequestDetail
		e := db.Transaction(func(tx *gorm.DB) error {
			var e error
			out, e = buyback.ReadRequest(ctx, tx, shop, account, request)
			return e
		})
		return out, e
	}
	current, e := customers.Buyback(ctx, token, store, id)
	if e != nil || current.ID != id || len(current.Photos) != 0 || current.History == nil || current.DeletedAt != nil || current.ClosedAt != nil {
		t.Fatal("authenticated buyback detail")
	}
	for _, v := range [][3]string{{"cust_buyback_second", store, id}, {"cust_buyback_first", "missing-shop", id}, {"cust_buyback_first", store, "missing"}} {
		if _, e := read(v[0], v[1], v[2]); !errors.Is(e, buyback.ErrMissing) {
			t.Fatal("buyback ownership leaked")
		}
	}
	// Historical matching customer IDs alone cannot create ownership.
	exec("UPDATE customer_buyback_submissions SET customer_id='cust_buyback_second' WHERE request_id=?", id)
	if _, e := read("cust_buyback_first", store, id); !errors.Is(e, buyback.ErrMissing) {
		t.Fatal("historical ID granted ownership")
	}
	exec("UPDATE customer_buyback_submissions SET customer_id='cust_buyback_first' WHERE request_id=?", id)
	exec("UPDATE buyback_requests SET status='final_offer',final_amount=123.45,raw_final_amount='{\"value\":\"123.450000000000000001\",\"precision\":100}',history='[{\"at\":\"2026-10-30T00:00:00Z\",\"action\":\"review_photo\",\"actor\":\"platform\",\"status\":\"final_offer\",\"note\":\"public note\",\"photo_id\":\"bbp_new\"}]' WHERE id=?", id)
	// Malformed raw fails closed; valid authoritative raw retains source precision.
	exec("UPDATE buyback_requests SET raw_final_amount='{}' WHERE id=?", id)
	if _, e := customers.Buyback(ctx, token, store, id); e == nil {
		t.Fatal("malformed raw final accepted")
	}
	exec(`UPDATE buyback_requests SET final_amount=123.450000000000000001,raw_final_amount='{"value":"123.450000000000000001","precision":100}' WHERE id=?`, id)
	item := current.Items[0].ID
	for _, p := range []struct{ id, kind, status, stamp string }{{"bbp_old", "front", "approved", "2026-10-29 00:00:00"}, {"bbp_back", "back", "pending", "2026-10-29 01:00:00"}, {"bbp_a", "front", "approved", "2026-10-30 00:00:00"}, {"bbp_z", "front", "retake", "2026-10-30 00:00:00"}, {"bbp_deleted", "front", "approved", "2026-10-30 01:00:00"}} {
		exec("INSERT INTO buyback_photos(id,request_id,item_id,photo_type,uploaded_at,storage_key,mime_type,review_status,review_note,created_at,updated_at) VALUES(?,?,?,?,?,'PRIVATE_STORAGE_PATH','image/jpeg',?,'public review',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", p.id, id, item, p.kind, p.stamp, p.status)
	}
	exec("UPDATE buyback_photos SET deleted_at=UTC_TIMESTAMP(6) WHERE id='bbp_deleted'")
	current, e = customers.Buyback(ctx, token, store, id)
	if e != nil || current.Status != "final_offer" || current.FinalAmount == nil || string(*current.FinalAmount) != "123.450000000000000001" || len(current.Photos) != 2 || current.Photos[0].ID != "bbp_z" || current.Photos[0].ReviewStatus != "retake" || current.Photos[1].ID != "bbp_back" {
		t.Fatal("current buyback projection or latest photos")
	}
	body, _ := json.Marshal(current)
	if !strings.Contains(string(body), "public note") || !strings.Contains(string(body), "photo_id") || strings.Contains(string(body), "storage_key") || strings.Contains(string(body), "PRIVATE_STORAGE_PATH") || strings.Contains(string(body), "customer_id") || strings.Contains(string(body), "submission_key") {
		t.Fatal("buyback public detail privacy/history")
	}
	for _, c := range []struct {
		path, session, origin string
		status                int
	}{{id, token, "", 200}, {id, token, "https://evil.example", 403}, {id, "", "", 401}, {"missing", token, "", 404}, {id + "?offset=0", token, "", 404}, {id + "?", token, "", 404}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "https://shop.example/api/store/customer/buyback/requests/"+c.path, nil)
		if c.session != "" {
			r.Header.Set("Cookie", "__Host-retail_customer_session="+c.session)
		}
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		router.ServeHTTP(w, r)
		if w.Code != c.status {
			t.Fatalf("buyback detail HTTP got %d want %d", w.Code, c.status)
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("buyback detail cached")
		}
	}
	exec("UPDATE buyback_requests SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", id)
	if _, e := customers.Buyback(ctx, token, store, id); !errors.Is(e, buyback.ErrMissing) {
		t.Fatal("soft deleted buyback visible")
	}
	exec("UPDATE buyback_requests SET deleted_at=NULL WHERE id=?", id)
	exec("UPDATE buyback_request_runtime SET enabled=FALSE WHERE id=1")
	if _, e := customers.Buyback(ctx, token, store, id); e != nil {
		t.Fatal("read depends on write gate")
	}
	exec("UPDATE buyback_request_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE customer_accounts SET enabled=FALSE WHERE id='cust_buyback_first'")
	if _, e := customers.Buyback(ctx, token, store, id); !errors.Is(e, customer.ErrUnauthorized) {
		t.Fatal("disabled account read")
	}
	exec("UPDATE customer_accounts SET enabled=TRUE WHERE id='cust_buyback_first'")
	checkBuybackList(t, db, customers, router, token, store, id)
	t.Log(`BUYBACK_READ_REHEARSAL {"current_not_receipt":true,"exact_raw_amount":true,"explicit_account_and_store":true,"historical_match_denied":true,"soft_delete":true,"latest_photo_ties":true,"photo_storage_private":true,"history_note_and_photo":true,"http_session_origin_query":true,"write_gate_independent":true,"disabled_customer":true}`)
}

func checkBuybackList(t *testing.T, db *gorm.DB, customers *customer.Store, router *gin.Engine, token, store, id string) {
	t.Helper()
	ctx := context.Background()
	before, e := customers.Buybacks(ctx, token, store, 0)
	if e != nil || before.Count < 1 {
		t.Fatal("initial buyback list")
	}
	original, e := customers.Buyback(ctx, token, store, id)
	if e != nil {
		t.Fatal("list source")
	}
	for i := 0; i < 23; i++ {
		in := buyback.Submission{SubmissionKey: fmt.Sprintf("00000000-0000-4000-8000-%012d", i+100), ContactName: original.ContactName, Phone: original.Phone, ReturnAddress: original.ReturnAddress, ReferenceAcknowledged: true, PrivacyAcknowledged: true, Items: []buyback.SubmissionItem{{Model: original.Items[0].Model, Capacity: original.Items[0].Capacity, Assessment: original.Items[0].Assessment}}}
		var made buyback.CreatedRequest
		e := db.Transaction(func(tx *gorm.DB) error {
			var e error
			made, e = buyback.StageCreate(ctx, tx, store, "cust_buyback_first", in)
			return e
		})
		if e != nil {
			t.Fatal("list creation fixture")
		}
		status := "awaiting_photos"
		if i < 2 {
			status = "paid"
		}
		if i == 2 {
			status = "canceled"
		}
		if i == 3 {
			status = "returned"
		}
		currency := "eur"
		amount := "1.005"
		if i == 1 {
			currency = "usd"
			amount = "2.005"
		}
		if db.Exec("UPDATE buyback_requests SET status=?,currency_code=?,final_amount=?,raw_final_amount=? WHERE id=?", status, currency, amount, `{"value":"`+amount+`","precision":100}`, made.ID).Error != nil {
			t.Fatal("list summary fixture")
		}
	}
	first, e := customers.Buybacks(ctx, token, store, 0)
	if e != nil || first.Count != before.Count+23 || len(first.Requests) != 20 || first.Limit != 20 || first.Offset != 0 || first.Summary.Recycled != 2 || first.Summary.Processing != before.Summary.Processing+19 || len(first.Summary.Received) != 2 || first.Summary.Received[0].CurrencyCode != "EUR" || first.Summary.Received[0].Amount != 1.01 || first.Summary.Received[1].CurrencyCode != "USD" || first.Summary.Received[1].Amount != 2.01 {
		t.Fatalf("buyback summary: err=%v count=%d before=%d page=%d summary=%+v before-summary=%+v",e,first.Count,before.Count,len(first.Requests),first.Summary,before.Summary)
	}
	second, e := customers.Buybacks(ctx, token, store, 20)
	if e != nil || len(second.Requests) != int(first.Count)-20 || second.Summary.Recycled != 2 || second.Summary.Processing != first.Summary.Processing {
		t.Fatal("buyback summary was page scoped")
	}
	seen := map[string]bool{}
	for _, v := range first.Requests {
		seen[v.ID] = true
	}
	for _, v := range second.Requests {
		if seen[v.ID] {
			t.Fatal("buyback pagination repeated request")
		}
	}
	empty, e := customers.Buybacks(ctx, token, store, 100000)
	if e != nil || len(empty.Requests) != 0 || empty.Count != first.Count || empty.Summary.Recycled != 2 {
		t.Fatal("buyback empty page lost summary")
	}
	for _, offset := range []int{-1, 100001} {
		if _, e := customers.Buybacks(ctx, token, store, offset); !errors.Is(e, customer.ErrInvalid) {
			t.Fatal("buyback invalid offset")
		}
	}
	for _, c := range []struct {
		query  string
		status int
	}{{"", 200}, {"?offset=20", 200}, {"?offset=100000", 200}, {"?offset=01", 400}, {"?offset=100001", 400}, {"?", 400}, {"?offset=0&offset=20", 400}, {"?customer_id=foreign", 400}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "https://shop.example/api/store/customer/buyback/requests"+c.query, nil)
		r.Header.Set("Cookie", "__Host-retail_customer_session="+token)
		router.ServeHTTP(w, r)
		if w.Code != c.status {
			t.Fatalf("buyback list HTTP got %d want %d", w.Code, c.status)
		}
	}
	t.Log(`BUYBACK_LIST_REHEARSAL {"page_size":20,"added_requests":23,"global_summary_all_pages":true,"paid_recycled_two":true,"returned_canceled_excluded":true,"currency_group_sort":true,"original_epsilon_rounding":true,"no_repeated_page_ids":true,"empty_page_keeps_summary":true,"strict_offsets":true}`)
}
