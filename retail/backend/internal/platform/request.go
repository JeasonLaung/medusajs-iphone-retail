package platform

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const requestIDKey = "retail.request_id"
const MaxJSONBodyBytes int64 = 1 << 20

// One device image, encoded as base64, plus bounded JSON metadata.
const MaxDevicePhotoBodyBytes int64 = (4 << 20) + 8192

// RequestBoundary replaces Gin's default access/recovery loggers. Only the
// matched route template is logged; unmatched paths, parameters, queries,
// headers, bodies, c.Errors and panic values never reach the logger.
// All incoming request IDs are ignored because no trusted proxy is configured.
func RequestBoundary(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		started := time.Now()
		id, err := uuid.NewRandom()
		if err != nil {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		requestID := "req_" + id.String()
		c.Set(requestIDKey, requestID)
		c.Header("X-Request-ID", requestID)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		if strings.HasPrefix(c.FullPath(), "/api/store/customer/") {
			c.Header("Cache-Control", "private, no-store")
			c.Header("Referrer-Policy", "no-referrer")
		}
		switch c.FullPath() {
		case "/api/store/orders/:id/confirmation", "/api/store/carts/:id/complete", "/api/store/carts/:id/payment-status", "/api/store/carts/:id/payment-attempts", "/api/store/carts/:id/shipping-methods", "/api/store/carts/:id/contacts", "/api/store/cart-sessions", "/api/store/carts", "/api/store/carts/:id", "/api/store/carts/:id/amounts", "/api/store/carts/:id/line-items":
			// Also applies to the body-size rejection before route middleware.
			c.Header("Cache-Control", "private, no-store")
		}
		panicked := false
		defer func() {
			if recover() != nil {
				panicked = true
				if !c.Writer.Written() {
					Fail(c, 500, "internal_error", "Unable to complete the request")
				}
				c.Abort()
			}
			route := c.FullPath()
			if route == "" {
				route = "unmatched"
			}
			method := c.Request.Method
			switch method {
			case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE":
			default:
				method = "OTHER"
			}
			logger.Info("http_request", "request_id", requestID, "method", method, "route", route,
				"status", c.Writer.Status(), "duration_ms", time.Since(started).Milliseconds(), "panic", panicked)
		}()
		limit := MaxJSONBodyBytes
		if c.Request.Method == http.MethodPost && (c.FullPath() == "/api/admin/device-photos" || c.FullPath() == "/api/admin/warehouse/locations/:location_id/device-intake/:category/photos") {
			limit = MaxDevicePhotoBodyBytes
		}
		if c.Request.ContentLength > limit {
			Fail(c, http.StatusRequestEntityTooLarge, "request_too_large", "Request body is too large")
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}

func PrivateResponse(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Next()
}
