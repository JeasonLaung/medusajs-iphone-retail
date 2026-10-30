package customer

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
)

var noticeOffset = regexp.MustCompile(`^offset=(0|[1-9][0-9]{0,5})$`)

func (h *HTTPHandler) ListNotices(c *gin.Context) {
	if !h.originOK(c, false) {
		return
	}
	offset := 0
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		if !noticeOffset.MatchString(c.Request.URL.RawQuery) {
			h.fail(c, ErrInvalid)
			return
		}
		offset, _ = strconv.Atoi(c.Query("offset"))
		if offset > 100000 {
			h.fail(c, ErrInvalid)
			return
		}
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := h.store.Notices(ctx, token, offset)
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(200, out)
}
func (h *HTTPHandler) MarkNoticesRead(c *gin.Context) {
	fields, ok := h.input(c, "id")
	if !ok || !h.originOK(c, true) {
		return
	}
	if id, present := fields["id"]; present && !validNoticeID(id) {
		h.fail(c, ErrInvalid)
		return
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if e := h.sessions.VerifyCSRF(ctx, sessionstore.Customer, token, customerCSRF(c)); e != nil {
		h.fail(c, e)
		return
	}
	if e := h.store.ReadNotices(ctx, token, fields["id"]); e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(200, gin.H{"success": true})
}
