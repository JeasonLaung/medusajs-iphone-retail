package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestContextDiagnosticsPreserveStatusAndPrivacy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, state := range []string{"active", "canceled", "deadline_exceeded"} {
		t.Run(state, func(t *testing.T) {
			var logs bytes.Buffer
			r := gin.New()
			r.Use(RequestBoundary(slog.New(slog.NewJSONHandler(&logs, nil))))
			r.GET("/fixture/:id", func(c *gin.Context) {
				Fail(c, 503, "fixture.unavailable", "Unavailable")
			})
			ctx := context.Background()
			if state == "canceled" {
				var cancel context.CancelCauseFunc
				ctx, cancel = context.WithCancelCause(ctx)
				cancel(context.DeadlineExceeded) // Cause must not override Err's classification.
			} else if state == "deadline_exceeded" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
			}
			req := httptest.NewRequest("GET", "/fixture/private-row?token=private-token", nil).WithContext(ctx)
			req.Header.Set("X-Request-ID", "private-forged-id")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			var record map[string]any
			if json.Unmarshal(logs.Bytes(), &record) != nil || record["request_context"] != state || record["status"] != float64(503) || w.Code != 503 {
				t.Fatal("request context or original failure status lost")
			}
			if record["route"] != "/fixture/:id" || record["request_id"] != w.Header().Get("X-Request-ID") || strings.Contains(logs.String(), "private-") {
				t.Fatal("private request material leaked or correlation lost")
			}
		})
	}
}
