package identity_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/buyback"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func checkBuybackPhotos(t *testing.T, db *gorm.DB, customers *customer.Store, router *gin.Engine, token, csrf, store string, in buyback.Submission) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("photo fixture SQL")
		}
	}
	in.SubmissionKey = "00000000-0000-4000-8000-000000009999"
	request, e := customers.CreateBuyback(ctx, token, store, in)
	if e != nil {
		t.Fatal("photo request")
	}
	id, item := request.ID, request.Items[0].ID
	upload := buyback.PhotoUpload{ItemID: item, PhotoType: "front", MimeType: "image/jpeg", Content: "/9j/"}
	photo, e := customers.UploadBuybackPhoto(ctx, token, store, id, upload)
	if e != nil || photo.ReviewStatus != "pending" || photo.ID == "" {
		t.Fatal("photo upload")
	}
	got, e := customers.BuybackPhoto(ctx, token, store, id, photo.ID)
	if e != nil || got.Content != "/9j/" || got.MimeType != "image/jpeg" {
		t.Fatal("photo bytes")
	}
	large := make([]byte, buyback.MaxPhotoBytes)
	copy(large, []byte{255, 216, 255})
	largeInput := upload
	largeInput.PhotoType = "back"
	largeInput.Content = base64.StdEncoding.EncodeToString(large)
	largeRaw, _ := json.Marshal(largeInput)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "https://shop.example/api/store/customer/buyback/requests/"+id+"/photos", strings.NewReader(string(largeRaw)))
	r.Header.Set("Origin", "https://shop.example")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", csrf)
	r.Header.Set("Cookie", "__Host-retail_customer_session="+token)
	router.ServeHTTP(w, r)
	var created struct{ Photo buyback.PublicPhoto }
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &created) != nil || created.Photo.ID == "" {
		t.Fatal("3 MiB HTTP photo upload")
	}
	largeRead, e := customers.BuybackPhoto(ctx, token, store, id, created.Photo.ID)
	if e != nil || largeRead.Content != largeInput.Content {
		t.Fatal("3 MiB private SQL bytes roundtrip")
	}
	if db.Transaction(func(tx *gorm.DB) error {
		_, e := buyback.ReadPhoto(ctx, tx, store, "cust_buyback_second", id, photo.ID)
		if !errors.Is(e, buyback.ErrMissing) {
			t.Fatal("cross account photo read")
		}
		_, e = buyback.StagePhotoUpload(ctx, tx, store, "cust_buyback_second", id, upload)
		if !errors.Is(e, buyback.ErrMissing) {
			t.Fatal("cross account photo write")
		}
		return nil
	}) != nil {
		t.Fatal("photo ownership fixture")
	}
	exec("UPDATE buyback_photo_bytes SET sha256=REPEAT('0',64) WHERE photo_id=?", photo.ID)
	if _, e := customers.BuybackPhoto(ctx, token, store, id, photo.ID); e == nil {
		t.Fatal("corrupted photo served")
	}
	exec("UPDATE buyback_photo_bytes SET sha256=SHA2(content,256) WHERE photo_id=?", photo.ID)
	exec("UPDATE buyback_photos SET review_status='expired' WHERE id=?", photo.ID)
	if _, e := customers.BuybackPhoto(ctx, token, store, id, photo.ID); !errors.Is(e, buyback.ErrPhotoMissing) {
		t.Fatal("expired photo served")
	}
	exec("UPDATE buyback_photos SET review_status='pending' WHERE id=?", photo.ID)
	for _, status := range []string{"photo_review", "in_transit", "received", "final_offer", "accepted", "paid", "return_requested", "returning", "returned", "canceled"} {
		exec("UPDATE buyback_requests SET status=? WHERE id=?", status, id)
		if _, e := customers.UploadBuybackPhoto(ctx, token, store, id, upload); !errors.Is(e, buyback.ErrPhotoState) {
			t.Fatal("closed state upload")
		}
	}
	exec("UPDATE buyback_requests SET status='ready_to_ship' WHERE id=?", id)
	if _, e := customers.UploadBuybackPhoto(ctx, token, store, id, upload); !errors.Is(e, buyback.ErrPhotoState) {
		t.Fatal("front upload after approval")
	}
	for _, kind := range []string{"packaging", "sealed"} {
		v := upload
		v.PhotoType = kind
		if _, e := customers.UploadBuybackPhoto(ctx, token, store, id, v); e != nil {
			t.Fatal("shipping photo refused")
		}
	}
	exec("UPDATE buyback_requests SET status='awaiting_photos' WHERE id=?", id)
	bad := upload
	bad.ItemID = "foreign-item"
	if _, e := customers.UploadBuybackPhoto(ctx, token, store, id, bad); !errors.Is(e, buyback.ErrPhotoMissing) {
		t.Fatal("foreign item upload")
	}
	// A failure after both inserts must leave neither a photo record nor bytes.
	var before, after int64
	db.Table("buyback_photos").Where("request_id=?", id).Count(&before)
	late := errors.New("late photo failure")
	e = db.Transaction(func(tx *gorm.DB) error {
		if _, e := buyback.StagePhotoUpload(ctx, tx, store, "cust_buyback_first", id, upload); e != nil {
			return e
		}
		return late
	})
	db.Table("buyback_photos").Where("request_id=?", id).Count(&after)
	if !errors.Is(e, late) || before != after {
		t.Fatal("late photo rollback")
	}
	var orphan int64
	db.Raw("SELECT COUNT(*) FROM buyback_photo_bytes b LEFT JOIN buyback_photos p ON p.id=b.photo_id WHERE p.id IS NULL").Scan(&orphan)
	if orphan != 0 {
		t.Fatal("orphan photo bytes")
	}
	// Fill to 29; four simultaneous attempts have exactly one remaining slot.
	for n := before; n < 29; n++ {
		if _, e := customers.UploadBuybackPhoto(ctx, token, store, id, upload); e != nil {
			t.Fatal("photo quota fixture")
		}
	}
	var wg sync.WaitGroup
	results := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := customers.UploadBuybackPhoto(ctx, token, store, id, upload)
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	wins, denied := 0, 0
	for e := range results {
		if e == nil {
			wins++
		} else if errors.Is(e, buyback.ErrPhotoLimit) {
			denied++
		} else {
			t.Fatal("unexpected concurrent photo result")
		}
	}
	if wins != 1 || denied != 3 {
		t.Fatal("photo quota race")
	}
	call := func(method, path, body, session, proof string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, "https://shop.example/api/store/customer/buyback/requests/"+id+path, strings.NewReader(body))
		r.Header.Set("Origin", "https://shop.example")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", proof)
		if session != "" {
			r.Header.Set("Cookie", "__Host-retail_customer_session="+session)
		}
		router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("photo HTTP got %d want %d", w.Code, want)
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") || strings.Contains(w.Body.String(), "storage_key") {
			t.Fatal("photo privacy")
		}
	}
	raw, _ := json.Marshal(upload)
	call("GET", "/photos/"+photo.ID, "", token, "", 200)
	call("GET", "/photos/missing", "", token, "", 404)
	call("GET", "/photos/"+photo.ID+"?x=1", "", token, "", 404)
	call("POST", "/photos", string(raw), token, csrf, 409)
	call("POST", "/photos", string(raw), token, "", 403)
	call("POST", "/photos", string(raw), "", csrf, 401)
	call("POST", "/photos", strings.Replace(string(raw), `"content":`, `"content":"/9j/","content":`, 1), token, csrf, 400)
	exec("UPDATE buyback_request_runtime SET enabled=FALSE WHERE id=1")
	if _, e := customers.UploadBuybackPhoto(ctx, token, store, id, upload); e == nil {
		t.Fatal("disabled writes")
	}
	if _, e := customers.BuybackPhoto(ctx, token, store, id, photo.ID); e != nil {
		t.Fatal("photo read depends on write gate")
	}
	exec("UPDATE buyback_request_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE buyback_requests SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", id)
	if _, e := customers.BuybackPhoto(ctx, token, store, id, photo.ID); !errors.Is(e, buyback.ErrMissing) {
		t.Fatal("deleted request photos visible")
	}
	t.Log(`BUYBACK_PHOTO_REHEARSAL {"private_bytes_roundtrip":true,"corrupt_hash_denied":true,"expired_denied":true,"ten_closed_states_denied":true,"shipping_packaging_sealed_only":true,"foreign_item_denied":true,"late_rollback":true,"quota_30":true,"concurrent_last_slot":[1,3],"http_session_csrf_query_duplicate":true,"write_gate_independent_read":true,"deleted_request_denied":true}`)
}
