package platform

import (
	"github.com/gin-gonic/gin"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPhotoBodyLimitIsRouteAndMethodSpecific(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestBoundary(slog.New(slog.NewTextHandler(io.Discard, nil))))
	handler := func(c *gin.Context) {
		if _, e := io.Copy(io.Discard, c.Request.Body); e != nil {
			c.Status(413)
			return
		}
		c.Status(200)
	}
	r.POST("/api/admin/device-photos", handler)
	r.GET("/api/admin/device-photos", handler)
	r.POST("/api/admin/products", handler)
	for _, v := range []struct {
		method, path string
		size         int64
		status       int
	}{{"POST", "/api/admin/device-photos", MaxJSONBodyBytes + 1, 200}, {"POST", "/api/admin/products", MaxJSONBodyBytes + 1, 413}, {"GET", "/api/admin/device-photos", MaxJSONBodyBytes + 1, 413}, {"POST", "/api/admin/device-photos", MaxDevicePhotoBodyBytes + 1, 413}} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(v.method, v.path, strings.NewReader(strings.Repeat("x", int(v.size))))
		r.ServeHTTP(w, req)
		if w.Code != v.status {
			t.Fatalf("%s %s: %d", v.method, v.path, w.Code)
		}
	}
}
