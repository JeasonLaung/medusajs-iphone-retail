package platform

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Failure struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func Fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, struct {
		Error Failure `json:"error"`
	}{Failure{Code: code, Message: message, RequestID: c.GetString(requestIDKey)}})
}
func DBError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, 404, "not_found", "Resource not found")
		return
	}
	Fail(c, 500, "internal_error", "Unable to complete the request")
}

// InvalidJSON preserves request-size status even when the body was chunked
// and its length was unknown before the handler attempted to decode it.
func InvalidJSON(c *gin.Context, err error, code, message string) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		Fail(c, http.StatusRequestEntityTooLarge, "request_too_large", "Request body is too large")
		return
	}
	Fail(c, http.StatusBadRequest, code, message)
}

func GuardOrigin(c *gin.Context) {
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
		c.Next()
		return
	}
	// Browser writes are same-origin. Proxy configuration must preserve the public Host.
	origin := c.GetHeader("Origin")
	if origin != "" && origin != "http://"+c.Request.Host && origin != "https://"+c.Request.Host {
		Fail(c, 403, "invalid_origin", "Cross-origin writes are not allowed")
		return
	}
	if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
		Fail(c, 403, "invalid_origin", "Cross-site writes are not allowed")
		return
	}
	c.Next()
}
