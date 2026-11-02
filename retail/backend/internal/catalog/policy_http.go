package catalog

import (
	"context"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mime"
	"net/http"
	"time"
)

func policySettingsFailure(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errEditInput):
		platform.Fail(c, 400, "policies.invalid_input", "Invalid policy settings")
	case errors.Is(err, authorization.ErrDenied):
		platform.Fail(c, 403, "authorization.denied", "Current purchase policy permission required")
	case errors.Is(err, ErrEditConflict):
		platform.Fail(c, 409, "policies.conflict", "Policy settings changed; reload before saving")
	case errors.Is(err, gorm.ErrRecordNotFound):
		platform.Fail(c, 404, "policies.not_found", "Store not found")
	default:
		platform.Fail(c, 503, "policies.unavailable", "Policy settings unavailable; retry the same operation")
	}
}

func (h StoreHandler) PolicySettingsHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		policySettingsFailure(c, errEditInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if c.Request.Method == http.MethodGet {
		out, e := h.ReadPolicySettings(ctx, actor)
		if e != nil {
			policySettingsFailure(c, e)
			return
		}
		c.JSON(200, out)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "policies.content_type", "JSON required")
		return
	}
	in, e := decodePolicyCommand(c.Request.Body)
	if e != nil {
		platform.InvalidJSON(c, e, "policies.invalid_input", "Invalid policy settings")
		return
	}
	out, e := h.SavePolicySettings(ctx, actor, in)
	if e != nil {
		policySettingsFailure(c, e)
		return
	}
	c.JSON(200, out)
}
