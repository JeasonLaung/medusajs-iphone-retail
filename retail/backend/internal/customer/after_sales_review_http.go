package customer

import (
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ReviewHandler struct{ DB *gorm.DB }

func (h ReviewHandler) HTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.required", "Sign in required")
		return
	}
	var in *ReviewInput
	if c.Request.Method == "POST" {
		// Reuse the strict string-only boundary (size/type/duplicate/unknown fields).
		values, ok := (&HTTPHandler{}).input(c, "operation_id", "expected_revision", "request_id", "status")
		if !ok {
			return
		}
		if len(values) != 4 {
			platform.Fail(c, 400, "after_sales.invalid", "Invalid review")
			return
		}
		in = &ReviewInput{OperationID: values["operation_id"], ExpectedRevision: values["expected_revision"], RequestID: values["request_id"], Status: values["status"]}
	}
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		platform.Fail(c, 400, "after_sales.invalid", "Invalid query")
		return
	}
	if h.DB == nil {
		platform.Fail(c, 503, "after_sales.unavailable", "After-sales review unavailable")
		return
	}
	var out ReviewContext
	e := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var e error
		out, e = ReviewAfterSalesTx(c.Request.Context(), tx, actor, c.Param("id"), in)
		return e
	})
	if e != nil {
		status := 503
		if errors.Is(e, ErrInvalid) {
			status = 400
		} else if errors.Is(e, ErrAfterSalesConflict) {
			status = 409
		} else if errors.Is(e, authorization.ErrDenied) {
			status = 403
		} else if errors.Is(e, gorm.ErrRecordNotFound) {
			status = 404
		}
		platform.Fail(c, status, "after_sales.unavailable", "After-sales review unavailable")
		return
	}
	c.JSON(200, out)
}
