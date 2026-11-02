package refunds

import (
	"context"
	"errors"
	"mime"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
)

func (h Handler) CancellationHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.required", "Sign in required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	var out CancellationView
	var err error
	status := 200
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		err = ErrInput
	} else if c.Request.Method == "GET" {
		out, err = h.Service.ReadCancellation(ctx, actor, c.Param("id"))
		out.Enabled = out.Enabled && h.Enabled
		out.Eligible = out.Eligible && h.Enabled
	} else if !h.Enabled {
		err = ErrUnavailable
	} else {
		media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if e != nil || media != "application/json" {
			platform.Fail(c, 415, "order_cancellation.content_type", "JSON required")
			return
		}
		var fields map[string]string
		if previous := c.Param("refund_operation_id"); previous != "" {
			fields, err = decodeStrings(c.Request.Body, []string{"operation_id"}, nil)
			if err == nil {
				out, err = h.Service.RetryCancellationRefund(ctx, actor, c.Param("id"), previous, fields["operation_id"])
			}
		} else {
			fields, err = decodeStrings(c.Request.Body, []string{"operation_id", "expected_version", "reason"}, nil)
			if err == nil {
				out, err = h.Service.CancelOrder(ctx, actor, c.Param("id"), CancellationInput{OperationID: fields["operation_id"], ExpectedVersion: fields["expected_version"], Reason: fields["reason"]})
			}
		}
		status = 202
	}
	if err == nil {
		c.JSON(status, out)
		return
	}
	status = 503
	switch {
	case errors.Is(err, ErrInput):
		status = 400
	case errors.Is(err, authorization.ErrDenied):
		status = 403
	case errors.Is(err, ErrConflict):
		status = 409
	}
	platform.Fail(c, status, "order_cancellation.unavailable", "Order cancellation unavailable; reload the order and check its payments, fulfillment and permissions")
}
