package platform

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, Failure{code, message})
}
func DBError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, 404, "not_found", "Resource not found")
		return
	}
	_ = c.Error(err)
	Fail(c, 500, "internal_error", "Unable to complete the request")
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
