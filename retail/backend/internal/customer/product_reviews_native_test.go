package customer

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/productreviews"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestNativeProductReviewCustomerHTTP(t *testing.T) {
	if os.Getenv("PRODUCT_REVIEWS_NATIVE_REHEARSAL") != "1" {
		t.Skip("dedicated product review MySQL/Redis fixture required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal("fixture unavailable")
	}
	pool, _ := db.DB()
	defer pool.Close()
	var database string
	if db.Raw("SELECT DATABASE()").Scan(&database).Error != nil || !strings.HasPrefix(database, "eurorefurb_product_create_") {
		t.Fatal("owned fixture required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	prefix := os.Getenv("SESSION_REDIS_PREFIX")
	if !strings.HasPrefix(prefix, "eurorefurb:product-create-") {
		t.Fatal("owned Redis namespace required")
	}
	sessions, redis, e := sessionstore.Open(ctx, os.Getenv("SESSION_REDIS_URL"), prefix)
	if e != nil {
		t.Fatal("fixture Redis unavailable")
	}
	defer redis.Close()
	// This test deletes only capabilities that it issued, never a namespace scan.
	ring, e := protected.New("review-test", map[string][]byte{"review-test": bytes.Repeat([]byte{45}, 32)})
	if e != nil {
		t.Fatal(e)
	}
	var store, product string
	if db.Table("commerce_stores").Select("id").Where("deleted_at IS NULL").Order("id").Limit(1).Scan(&store).Error != nil || store == "" {
		t.Fatal("store required")
	}
	if db.Table("catalog_products p").Select("p.id").Joins("JOIN catalog_product_channels c ON c.product_id=p.id AND c.deleted_at IS NULL").Joins("JOIN commerce_stores s ON s.default_sales_channel_id=c.sales_channel_id AND s.id=?", store).Where("p.status='published' AND p.deleted_at IS NULL").Order("p.id").Limit(1).Scan(&product).Error != nil || product == "" {
		t.Fatal("product required")
	}
	if db.Exec("UPDATE product_review_runtime SET reads_enabled=TRUE,writes_enabled=TRUE WHERE id=1").Error != nil {
		t.Fatal("test gate failed")
	}
	stamp := time.Now().UTC()
	account := record{Account: Account{ID: "customer-review_" + uuid.NewString(), Email: uuid.NewString() + "@example.test", FirstName: "HTTP first", LastName: "Private surname", VerifiedAt: &stamp, CreatedAt: stamp, UpdatedAt: stamp}, PasswordHash: "fixture-not-a-login-hash", Enabled: true, AuthVersion: 1}
	if db.Create(&account).Error != nil {
		t.Fatal("fixture account failed")
	}
	issued, e := sessions.Issue(ctx, sessionstore.Customer, sessionstore.Principal{UserID: account.ID, Version: version(account)})
	if e != nil {
		t.Fatal("fixture session failed")
	}
	defer sessions.Revoke(context.Background(), sessionstore.Customer, issued.Token)
	adminRealm, e := sessions.Issue(ctx, sessionstore.Admin, sessionstore.Principal{UserID: account.ID, Version: version(account)})
	if e != nil {
		t.Fatal("admin realm fixture failed")
	}
	defer sessions.Revoke(context.Background(), sessionstore.Admin, adminRealm.Token)
	const origin = "http://127.0.0.1:61124"
	key := strings.Repeat("61", 32)
	secret, _ := hex.DecodeString(key)
	h, e := NewHTTP(db, HTTPOptions{Sessions: sessions, Ring: ring, Origin: origin, StoreID: store, BridgeKey: key})
	if e != nil {
		t.Fatal("customer HTTP setup failed")
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h.Register(router.Group("/api/store/customer"))
	body := func(op string) string {
		raw, _ := json.Marshal(productreviews.Submission{OperationID: op, ProductID: product, Rating: 4, Title: "HTTP review", Content: "The signed in customer writes this product review."})
		return string(raw)
	}
	request := func(method, path, raw, token, csrf, requestOrigin string, sign bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, origin+path, strings.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if requestOrigin != "" {
			req.Header.Set("Origin", requestOrigin)
		}
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "retail_customer_session", Value: token})
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if sign {
			timestamp := strconv.FormatInt(time.Now().Unix(), 10)
			sum := sha256.Sum256([]byte(raw))
			ip := "127.0.0.1"
			fields := []string{"retail-customer-bridge-v1", timestamp, ip, method, req.URL.RequestURI(), req.Host, req.Header.Get("Origin"), req.Header.Get("Cookie"), req.Header.Get("X-CSRF-Token"), hex.EncodeToString(sum[:])}
			mac := hmac.New(sha256.New, secret)
			mac.Write([]byte(strings.Join(fields, "\n")))
			req.Header.Set("X-Retail-Client-IP", ip)
			req.Header.Set("X-Retail-Bridge-Time", timestamp)
			req.Header.Set("X-Retail-Bridge-Proof", hex.EncodeToString(mac.Sum(nil)))
		}
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		if out.Header().Get("Cache-Control") != "private, no-store" || out.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("private response headers missing")
		}
		return out
	}
	const path = "/api/store/customer/product-reviews"
	operation := uuid.NewString()
	raw := body(operation)
	for _, tc := range []struct {
		name, token, csrf, origin string
		sign                      bool
		want                      int
	}{
		{"no bridge", issued.Token, issued.CSRF, origin, false, 403},
		{"no session", "", issued.CSRF, origin, true, 401},
		{"no csrf", issued.Token, "", origin, true, 403},
		{"wrong csrf", issued.Token, strings.Repeat("f", 64), origin, true, 403},
		{"wrong origin", issued.Token, issued.CSRF, "http://evil.example", true, 403},
		{"absent origin", issued.Token, issued.CSRF, "", true, 403},
		{"admin session", adminRealm.Token, adminRealm.CSRF, origin, true, 401},
	} {
		if out := request("POST", path, raw, tc.token, tc.csrf, tc.origin, tc.sign); out.Code != tc.want {
			t.Fatal("authority boundary status mismatch", tc.name, out.Code)
		}
	}
	first := request("POST", path, raw, issued.Token, issued.CSRF, origin, true)
	if first.Code != 201 {
		t.Fatal("valid signed review failed", first.Code)
	}
	var accepted struct{ Review productreviews.Result }
	if json.Unmarshal(first.Body.Bytes(), &accepted) != nil || accepted.Review.ID == "" || accepted.Review.Status != "pending" || accepted.Review.Replayed {
		t.Fatal("submission response invalid")
	}
	second := request("POST", path, raw, issued.Token, issued.CSRF, origin, true)
	var replay struct{ Review productreviews.Result }
	json.Unmarshal(second.Body.Bytes(), &replay)
	if second.Code != 201 || replay.Review.ID != accepted.Review.ID || !replay.Review.Replayed {
		t.Fatal("original request retry did not recover receipt")
	}
	if out := request("POST", path, body(uuid.NewString()), issued.Token, issued.CSRF, origin, true); out.Code != 409 || !strings.Contains(out.Body.String(), "review.already_submitted") {
		t.Fatal("second review not rejected")
	}
	purchases := request("GET", path+"/purchases?product_id="+product+"&offset=0", "", issued.Token, "", origin, true)
	if purchases.Code != 200 || !strings.Contains(purchases.Body.String(), accepted.Review.ID) || strings.Contains(purchases.Body.String(), account.Email) || strings.Contains(purchases.Body.String(), account.LastName) {
		t.Fatal("private purchases projection incorrect", purchases.Code)
	}
	for _, query := range []string{"?product_id=" + product + "&customer_id=other", "?product_id=" + product + "&offset=00", "?product_id=" + product + "&limit=100", "?product_id=" + product + "&product_id=other"} {
		if out := request("GET", path+"/purchases"+query, "", issued.Token, "", origin, true); out.Code != 400 {
			t.Fatal("ambiguous private query accepted", out.Code)
		}
	}
	if out := request("GET", "/api/store/customer/orders/order-foreign/product-reviews", "", issued.Token, "", origin, true); out.Code != 404 {
		t.Fatal("foreign order context exposed", out.Code)
	}
	// Current MySQL generation is checked after Redis resolution even for the
	// same already-committed operation; an existing receipt grants no authority.
	if db.Exec("UPDATE customer_accounts SET auth_version=auth_version+1 WHERE id=?", account.ID).Error != nil {
		t.Fatal("fixture revocation failed")
	}
	if out := request("POST", path, raw, issued.Token, issued.CSRF, origin, true); out.Code != 401 {
		t.Fatal("revoked account replay accepted", out.Code)
	}
	if out := request("GET", path+"/purchases?product_id="+product, "", issued.Token, "", origin, true); out.Code != 401 {
		t.Fatal("revoked account reads private context", out.Code)
	}
	var saved struct{ Author, Status string }
	if db.Table("product_reviews").Select("author,status").Where("id=?", accepted.Review.ID).Take(&saved).Error != nil || saved.Author != account.FirstName || saved.Status != "pending" {
		t.Fatal("server-owned author/pending snapshot changed")
	}
	t.Log(`PRODUCT_REVIEWS_CUSTOMER_HTTP {"real_mysql_redis":true,"signed_bridge":true,"origin_csrf":true,"admin_realm_denied":true,"original_retry":true,"current_auth_on_receipt":true,"private_queries":true,"browser":false}`)
}
