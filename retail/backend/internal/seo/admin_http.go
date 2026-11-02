package seo

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func adminFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrCommandInput):
		platform.Fail(c, 400, "seo.invalid_input", "Invalid SEO configuration")
	case errors.Is(err, authorization.ErrDenied):
		platform.Fail(c, 403, "authorization.denied", "Current SEO permission required")
	case errors.Is(err, ErrConflict):
		platform.Fail(c, 409, "seo.conflict", "SEO configuration changed; reload before editing")
	case errors.Is(err, ErrProtected):
		platform.Fail(c, 409, "seo.protected", "Protected historical SEO configuration requires reconciliation")
	case errors.Is(err, gorm.ErrRecordNotFound):
		platform.Fail(c, 404, "seo.not_found", "SEO record not found")
	default:
		platform.Fail(c, 503, "seo.unavailable", "SEO configuration unavailable; retry the same operation")
	}
}
func directoryQuery(raw string, resource string) (int, int, error) {
	limit, offset := 50, 0
	values, err := url.ParseQuery(raw)
	if err != nil || (resource == "settings" && len(values) != 0) {
		return 0, 0, ErrCommandInput
	}
	for key, entries := range values {
		if len(entries) != 1 || (key != "limit" && key != "offset") {
			return 0, 0, ErrCommandInput
		}
		n, e := strconv.Atoi(entries[0])
		if e != nil || strconv.Itoa(n) != entries[0] || (key == "limit" && (n < 1 || n > 100)) || (key == "offset" && (n < 0 || n > 100000)) {
			return 0, 0, ErrCommandInput
		}
		if key == "limit" {
			limit = n
		} else {
			offset = n
		}
	}
	return limit, offset, nil
}
func (s Store) RegisterAdmin(group *gin.RouterGroup) {
	for _, resource := range []string{"settings", "pages"} {
		resource := resource
		handler := func(c *gin.Context) {
			c.Header("Cache-Control", "private, no-store")
			value, _ := c.Get("admin_user")
			actor, ok := value.(identity.User)
			if !ok {
				platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
				return
			}
			ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
			defer cancel()
			if c.Request.Method == http.MethodGet {
				limit, offset, e := directoryQuery(c.Request.URL.RawQuery, resource)
				if e != nil || c.Request.URL.ForceQuery {
					adminFailure(c, ErrCommandInput)
					return
				}
				out, e := s.Directory(ctx, actor, resource, limit, offset)
				if e != nil {
					adminFailure(c, e)
					return
				}
				c.JSON(200, out)
				return
			}
			if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
				adminFailure(c, ErrCommandInput)
				return
			}
			media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
			if e != nil || media != "application/json" {
				platform.Fail(c, 415, "seo.content_type", "JSON required")
				return
			}
			raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 256*1024+1))
			if e != nil {
				platform.InvalidJSON(c, e, "seo.invalid_input", "Invalid SEO configuration")
				return
			}
			if len(raw) > 256*1024 {
				platform.Fail(c, 413, "request_too_large", "Request body is too large")
				return
			}
			in, e := decodeCommand(raw)
			if e != nil || in.Resource != resource {
				adminFailure(c, ErrCommandInput)
				return
			}
			out, e := s.Save(ctx, actor, in)
			if e != nil {
				adminFailure(c, e)
				return
			}
			c.JSON(200, out)
		}
		group.GET("/seo/"+resource, handler)
		group.POST("/seo/"+resource, handler)
	}
}
