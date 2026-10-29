package customer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestBridgeBindsObservedAddressAndExactRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := strings.Repeat("71", 32)
	secret, ok := bridgeKey(key)
	if !ok {
		t.Fatal("fixture key invalid")
	}
	handler := &HTTPHandler{bridgeSecret: secret}
	r := gin.New()
	r.Use(handler.bridge)
	r.POST("/api/store/customer/auth/login", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		c.JSON(200, gin.H{"ip": handler.clientIP(c), "body": string(body)})
	})
	sign := func(ip string, stamp int64) *http.Request {
		req := httptest.NewRequest("POST", "https://shop.example/api/store/customer/auth/login", strings.NewReader(`{"email":"fixture@example.invalid"}`))
		req.Header.Set("Origin", "https://shop.example")
		req.Header.Set("Cookie", "retail_customer_prelogin="+strings.Repeat("a", 64))
		req.Header.Set("X-CSRF-Token", strings.Repeat("b", 64))
		timestamp := strconv.FormatInt(stamp, 10)
		digest := sha256.Sum256([]byte(`{"email":"fixture@example.invalid"}`))
		fields := []string{"retail-customer-bridge-v1", timestamp, ip, req.Method, req.URL.RequestURI(), req.Host, req.Header.Get("Origin"), req.Header.Get("Cookie"), req.Header.Get("X-CSRF-Token"), hex.EncodeToString(digest[:])}
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte(strings.Join(fields, "\n")))
		req.Header.Set("X-Retail-Client-IP", ip)
		req.Header.Set("X-Retail-Bridge-Time", timestamp)
		req.Header.Set("X-Retail-Bridge-Proof", hex.EncodeToString(mac.Sum(nil)))
		return req
	}
	now := time.Now().Unix()
	good := sign("::ffff:192.0.2.5", now)
	good.Header.Set("X-Forwarded-For", "evil")
	out := httptest.NewRecorder()
	r.ServeHTTP(out, good)
	if out.Code != 200 || !strings.Contains(out.Body.String(), `"ip":"192.0.2.5"`) || !strings.Contains(out.Body.String(), "fixture@example.invalid") {
		t.Fatal("signed client address or restored body failed")
	}
	for _, edit := range []func(*http.Request){
		func(req *http.Request) { req.Header.Del("X-Retail-Bridge-Proof") },
		func(req *http.Request) { req.Header.Set("X-Retail-Client-IP", "192.0.2.6") },
		func(req *http.Request) { req.Header.Set("Origin", "https://evil.example") },
		func(req *http.Request) { req.Host = "evil.example" },
		func(req *http.Request) { req.Header.Set("Cookie", "other") },
		func(req *http.Request) { req.Header.Set("X-CSRF-Token", "other") },
		func(req *http.Request) {
			req.Header.Add("X-Retail-Bridge-Proof", req.Header.Get("X-Retail-Bridge-Proof"))
		},
		func(req *http.Request) { req.Body = io.NopCloser(strings.NewReader("changed")) },
	} {
		req := sign("192.0.2.5", now)
		edit(req)
		out := httptest.NewRecorder()
		r.ServeHTTP(out, req)
		if out.Code != 403 {
			t.Fatal("tampered bridge proof accepted")
		}
	}
	for _, stamp := range []int64{now - 31, now + 10} {
		out := httptest.NewRecorder()
		r.ServeHTTP(out, sign("192.0.2.5", stamp))
		if out.Code != 403 {
			t.Fatal("stale/future proof accepted")
		}
	}
	for _, ip := range []string{"not-an-ip", "fe80::1%zone"} {
		out := httptest.NewRecorder()
		r.ServeHTTP(out, sign(ip, now))
		if out.Code != 403 {
			t.Fatal("invalid client address accepted")
		}
	}
	for _, bad := range []string{"", strings.Repeat("0", 64), strings.Repeat("AA", 32), "short"} {
		if _, ok := bridgeKey(bad); ok {
			t.Fatal("invalid bridge key accepted")
		}
	}
}

func TestBridgeCrossLanguageVector(t *testing.T) {
	secret, _ := bridgeKey(strings.Repeat("71", 32))
	digest := sha256.Sum256([]byte("{}"))
	fields := []string{"retail-customer-bridge-v1", "1700000000", "192.0.2.5", "POST", "/api/store/customer/auth/login", "shop.example", "https://shop.example", "customer=fixture", strings.Repeat("b", 64), hex.EncodeToString(digest[:])}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strings.Join(fields, "\n")))
	if hex.EncodeToString(mac.Sum(nil)) != "805ac93b9fa7d70a0c3103f6829fa1e0a22270cae12b545da7a98f5bd6ef2513" {
		t.Fatal("Go/Node proof protocol differs")
	}
}
