package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/buyback"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func checkBuybackCreation(t *testing.T, db *gorm.DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("buyback create fixture")
		}
	}
	exec("UPDATE catalog_products SET title='iPhone 13',handle='swappie-iphone-13',status='published' WHERE id='public-a'")
	exec(`INSERT INTO catalog_option_values(id,value,option_id,created_at,updated_at) VALUES('create-256','256GB','public-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_product_option_values(id,product_product_option_id,product_option_value_id,created_at,updated_at) VALUES('create-member','public-membership','create-256',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, id := range []string{"cust_buyback_first", "cust_buyback_second"} {
		exec(`INSERT INTO customer_accounts(id,email,password_hash,first_name,last_name,enabled,verified_at,created_at,updated_at) VALUES(?,?,'unused','Fixture','Customer',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, id, id+"@example.invalid")
	}
	var store string
	if db.Table("commerce_stores").Select("id").Limit(1).Scan(&store).Error != nil || store == "" {
		t.Fatal("buyback store fixture")
	}
	raw := `{"submission_key":"123e4567-e89b-42d3-a456-426614174001","contact_name":"Customer","phone":"123456","return_address":{"name":"Customer","address_1":"Street 1","city":"City","postal_code":"001","country_code":"dk"},"reference_acknowledged":true,"privacy_acknowledged":true,"items":[{"model":"iphone-13","capacity":"256GB","assessment":{"battery":"90","checks":[true,true,true,true,true],"faults":["none"],"glass":["none"],"screen":"none","frame":"none","back":"none"}}]}`
	in, e := buyback.ParseSubmission([]byte(raw))
	if e != nil {
		t.Fatal("buyback create input")
	}
	call := func(account string, input buyback.Submission) (buyback.CreatedRequest, error) {
		var out buyback.CreatedRequest
		e := db.Transaction(func(tx *gorm.DB) error {
			var err error
			out, err = buyback.StageCreate(context.Background(), tx, store, account, input)
			return err
		})
		return out, e
	}
	if _, e := call("cust_buyback_first", in); !errors.Is(e, buyback.ErrUnavailable) {
		t.Fatal("disabled buyback write allowed")
	}
	exec("UPDATE buyback_request_runtime SET enabled=TRUE WHERE id=1")
	var wg sync.WaitGroup
	results := make(chan buyback.CreatedRequest, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := call("cust_buyback_first", in)
			if e != nil {
				failures <- e
			} else {
				results <- out
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for range failures {
		t.Fatal("concurrent buyback creation failed")
	}
	var first buyback.CreatedRequest
	for result := range results {
		if first.ID == "" {
			first = result
		}
		if result.ID != first.ID || result.Items[0].ID != first.Items[0].ID || !result.CreatedAt.Equal(first.CreatedAt) {
			t.Fatal("buyback replay duplicated result")
		}
	}
	if first.Email != "cust_buyback_first@example.invalid" || first.Status != "awaiting_photos" || first.ReferenceAmount == nil || *first.ReferenceAmount != "294" {
		t.Fatal("buyback creation snapshot")
	}
	var requests, receipts int64
	if db.Table("buyback_requests").Count(&requests).Error != nil || db.Table("customer_buyback_submissions").Count(&receipts).Error != nil || requests != 1 || receipts != 1 {
		t.Fatal("buyback concurrency duplicated records")
	}
	changed := in
	changed.Phone = "654321"
	if _, e := call("cust_buyback_first", changed); !errors.Is(e, buyback.ErrConflict) {
		t.Fatal("changed replay allowed")
	}
	other, e := call("cust_buyback_second", in)
	if e != nil || other.ID == first.ID || other.Email == first.Email {
		t.Fatal("cross account submission ownership")
	}
	exec("UPDATE buyback_request_runtime SET enabled=FALSE WHERE id=1")
	if replay, e := call("cust_buyback_first", in); e != nil || replay.ID != first.ID {
		t.Fatal("durable replay after write disable")
	}
	exec("UPDATE customer_accounts SET enabled=FALSE WHERE id='cust_buyback_first'")
	if _, e := call("cust_buyback_first", in); !errors.Is(e, buyback.ErrInput) {
		t.Fatal("disabled customer replay allowed")
	}
	exec("UPDATE customer_accounts SET enabled=TRUE WHERE id='cust_buyback_first'")
	exec("UPDATE buyback_request_runtime SET enabled=TRUE WHERE id=1")
	in.SubmissionKey = "123e4567-e89b-42d3-a456-426614174002"
	late := errors.New("late failure")
	e = db.Transaction(func(tx *gorm.DB) error {
		if _, e := buyback.StageCreate(context.Background(), tx, store, "cust_buyback_first", in); e != nil {
			return e
		}
		return late
	})
	if !errors.Is(e, late) {
		t.Fatal("late failure fixture")
	}
	var count int64
	if db.Table("customer_buyback_submissions").Where("submission_key=?", in.SubmissionKey).Count(&count).Error != nil || count != 0 {
		t.Fatal("late failure left receipt")
	}
	if db.Table("buyback_requests").Where("submission_key=?", in.SubmissionKey).Count(&count).Error != nil || count != 0 {
		t.Fatal("late failure left request")
	}
	ctx := context.Background()
	sessions, redis, e := sessionstore.Open(ctx, "redis://127.0.0.1:6379/0", "eurorefurb:buyback_create:v1:")
	if e != nil {
		t.Fatal("buyback session fixture")
	}
	defer redis.Close()
	ring, e := protected.New("fixture", map[string][]byte{"fixture": bytes.Repeat([]byte{47}, 32)})
	if e != nil {
		t.Fatal("buyback protection fixture")
	}
	customers, e := customer.New(db, ring, sessions, "https://shop.example")
	if e != nil {
		t.Fatal("buyback customer fixture")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte("fixture-password"), bcrypt.MinCost)
	if e != nil {
		t.Fatal("buyback password fixture")
	}
	exec("UPDATE customer_accounts SET password_hash=? WHERE id='cust_buyback_first'", string(hash))
	login, e := customers.Login(ctx, "cust_buyback_first@example.invalid", "fixture-password")
	if e != nil {
		t.Fatal("buyback customer login")
	}
	bound, e := customers.CreateBuyback(ctx, login.Session.Token, store, in)
	if e != nil || bound.Email != "cust_buyback_first@example.invalid" {
		t.Fatal("session bound buyback creation")
	}
	h, e := customer.NewHTTP(db, customer.HTTPOptions{Sessions: sessions, Ring: ring, Origin: "https://shop.example", Secure: true, StoreID: store})
	if e != nil {
		t.Fatal("buyback HTTP fixture")
	}
	router := gin.New()
	h.Register(router.Group("/api/store/customer"))
	body, _ := json.Marshal(in)
	for _, c := range []struct {
		csrf, origin, cookie, path, body string
		want                             int
	}{
		{login.Session.CSRF, "https://shop.example", login.Session.Token, "/api/store/customer/buyback/requests", string(body), 201},
		{"", "https://shop.example", login.Session.Token, "/api/store/customer/buyback/requests", string(body), 403},
		{login.Session.CSRF, "https://evil.example", login.Session.Token, "/api/store/customer/buyback/requests", string(body), 403},
		{login.Session.CSRF, "https://shop.example", "", "/api/store/customer/buyback/requests", string(body), 401},
		{login.Session.CSRF, "https://shop.example", login.Session.Token, "/api/store/customer/buyback/requests?customer_id=foreign", string(body), 404},
		{login.Session.CSRF, "https://shop.example", login.Session.Token, "/api/store/customer/buyback/requests", strings.Replace(string(body), `"phone":`, `"phone":"ignored","phone":`, 1), 400},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "https://shop.example"+c.path, strings.NewReader(c.body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", c.origin)
		r.Header.Set("X-CSRF-Token", c.csrf)
		if c.cookie != "" {
			r.Header.Set("Cookie", "__Host-retail_customer_session="+c.cookie)
		}
		router.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Fatalf("buyback HTTP got %d want %d", w.Code, c.want)
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("buyback response cache")
		}
		if c.want == 201 {
			var response struct{ Request buyback.CreatedRequest }
			if json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Request.ID != bound.ID || strings.Contains(w.Body.String(), "submission_key") || strings.Contains(w.Body.String(), "customer_id") {
				t.Fatal("buyback HTTP identity projection")
			}
		}
	}
	t.Log(`BUYBACK_CREATE_HTTP_REHEARSAL {"created_status":201,"csrf_required":true,"origin_required":true,"anonymous_denied":true,"query_denied":true,"duplicate_json_denied":true,"private_projection":true}`)
	checkBuybackRead(t, db, customers, router, login.Session.Token, store, bound.ID)
	checkBuybackPhotos(t, db, customers, router, login.Session.Token, login.Session.CSRF, store, in)
	checkBuybackCommands(t, db, customers, router, login.Session.Token, login.Session.CSRF, store, in)
	if e := customers.Logout(ctx, login.Session.Token); e != nil {
		t.Fatal("buyback customer logout")
	}
	if _, e := customers.CreateBuyback(ctx, login.Session.Token, store, in); !errors.Is(e, customer.ErrUnauthorized) {
		t.Fatal("revoked session read submission receipt")
	}
	if _, e := customers.Buyback(ctx, login.Session.Token, store, bound.ID); !errors.Is(e, customer.ErrUnauthorized) {
		t.Fatal("revoked buyback detail read")
	}
	if _, e := customers.UpdateBuyback(ctx, login.Session.Token, store, bound.ID, buyback.CustomerCommand{Action: "cancel"}); !errors.Is(e, customer.ErrUnauthorized) {
		t.Fatal("revoked buyback command")
	}
	if _, e := customers.CreateBuyback(ctx, "", store, in); !errors.Is(e, customer.ErrUnauthorized) {
		t.Fatal("anonymous buyback creation")
	}
	t.Log(`BUYBACK_CUSTOMER_CREATE_REHEARSAL {"real_redis_session":true,"customer_bound_creation":true,"revoked_replay_denied":true,"anonymous_denied":true}`)
	t.Log(`BUYBACK_CREATE_REHEARSAL {"concurrent_replays":8,"single_request_and_receipt":true,"changed_input_conflict":true,"cross_account_isolation":true,"disabled_gate":true,"disabled_customer":true,"late_failure_atomic":true,"customer_email_from_server":true}`)
}
