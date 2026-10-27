package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestRequestBoundaryAndPrivateDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	r := router(nil, slog.New(slog.NewJSONHandler(&logs, nil)))
	r.GET("/fixture/:id", func(c *gin.Context) {
		_ = c.Error(errors.New("private-database-parameter"))
		c.JSON(200, gin.H{"ok": true})
	})
	r.GET("/panic/:id", func(c *gin.Context) { panic("private-panic-password") })
	r.GET("/database-error/:id", func(c *gin.Context) { platform.DBError(c, errors.New("private-SQL-bindings")) })
	r.GET("/not-found/:id", func(c *gin.Context) { platform.DBError(c, gorm.ErrRecordNotFound) })
	r.GET("/stream-panic/:id", func(c *gin.Context) { c.String(200, "safe committed prefix"); panic("private-stream-panic") })
	seen := map[string]bool{}
	for _, sample := range []struct {
		method, path, body string
		status             int
		chunked            bool
	}{
		{"GET", "/fixture/private-row?token=private-query", "", 200, false},
		{"GET", "/private-unmatched?signature=private-signature", "", 404, false},
		{"DELETE", "/fixture/private-row", "", 405, false},
		{"GET", "/panic/private-row", "", 500, false},
		{"GET", "/database-error/private-row", "", 500, false},
		{"GET", "/not-found/private-row", "", 404, false},
		{"GET", "/stream-panic/private-row", "", 200, false},
		{"GET", "/health", "", 503, false},
		{"GET", "/api/admin/auth/me", "", 401, false},
		{"POST", "/api/admin/auth/login", "{", 400, false},
		{"POST", "/api/admin/auth/login", `{"email":"` + strings.Repeat("x", int(platform.MaxJSONBodyBytes)) + `","password":"private-password"}`, 413, false},
		{"POST", "/api/admin/auth/login", `{"email":"` + strings.Repeat("x", int(platform.MaxJSONBodyBytes)) + `","password":"private-password"}`, 413, true},
	} {
		logs.Reset()
		request := httptest.NewRequest(sample.method, sample.path, strings.NewReader(sample.body))
		if sample.chunked {
			request.ContentLength = -1
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Request-ID", "private-forged-request-id")
		request.Header.Set("Authorization", "Bearer private-authorization")
		request.Header.Set("Cookie", "private-cookie=private-value")
		request.Header.Set("Origin", "http://example.com")
		reply := httptest.NewRecorder()
		r.ServeHTTP(reply, request)
		if reply.Code != sample.status {
			t.Fatalf("unexpected response status for %s: %d", sample.path, reply.Code)
		}
		id := reply.Header().Get("X-Request-ID")
		if !strings.HasPrefix(id, "req_") || seen[id] || strings.Contains(id, "private") {
			t.Fatal("request identity not server-generated and unique")
		}
		seen[id] = true
		if reply.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("missing security header")
		}
		if strings.HasPrefix(sample.path, "/api/admin/") && sample.status != 413 && reply.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("private response cache policy missing")
		}
		if sample.status >= 400 {
			var result struct {
				Error platform.Failure `json:"error"`
			}
			if json.Unmarshal(reply.Body.Bytes(), &result) != nil || result.Error.Code == "" || result.Error.RequestID != id {
				t.Fatal("error envelope does not correlate")
			}
		}
		if bytes.Contains(reply.Body.Bytes(), []byte("private-")) || bytes.Contains(logs.Bytes(), []byte("private-")) {
			t.Fatal("private information disclosed")
		}
		var record map[string]any
		decoder := json.NewDecoder(bytes.NewReader(logs.Bytes()))
		if decoder.Decode(&record) != nil || record["request_id"] != id || record["status"] != float64(sample.status) {
			t.Fatal("incorrect access record")
		}
		if decoder.Decode(new(any)) != io.EOF {
			t.Fatal("duplicate or non-JSON access records")
		}
		for key := range record {
			switch key {
			case "time", "level", "msg", "request_id", "method", "route", "status", "duration_ms", "panic":
			default:
				t.Fatal("unreviewed log field")
			}
		}
		expectedRoute := "unmatched"
		if strings.HasPrefix(sample.path, "/fixture/") && sample.method == http.MethodGet {
			expectedRoute = "/fixture/:id"
		}
		if strings.HasPrefix(sample.path, "/panic/") {
			expectedRoute = "/panic/:id"
		}
		if strings.HasPrefix(sample.path, "/stream-panic/") {
			expectedRoute = "/stream-panic/:id"
		}
		if strings.HasPrefix(sample.path, "/database-error/") {
			expectedRoute = "/database-error/:id"
		}
		if strings.HasPrefix(sample.path, "/not-found/") {
			expectedRoute = "/not-found/:id"
		}
		if sample.path == "/health" || strings.HasPrefix(sample.path, "/api/admin/") {
			expectedRoute = sample.path
		}
		if record["route"] != expectedRoute {
			t.Fatal("route template not used")
		}
	}
	logs.Reset()
	request := httptest.NewRequest("private-method", "/private-target", nil)
	r.ServeHTTP(httptest.NewRecorder(), request)
	if bytes.Contains(logs.Bytes(), []byte("private-")) || !bytes.Contains(logs.Bytes(), []byte(`"method":"OTHER"`)) {
		t.Fatal("unrecognized method leaked")
	}
}
