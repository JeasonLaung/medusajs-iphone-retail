package buyback

import (
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"io"
	"net/url"
	"strconv"
	"strings"
)

type AdminHandler struct {
	DB      *gorm.DB
	StoreID string
}

func (h AdminHandler) Register(g *gin.RouterGroup) {
	g.GET("/buyback", h.HTTP)
	g.GET("/buyback/:id", h.HTTP)
	g.POST("/buyback/:id", h.HTTP)
	g.GET("/buyback/:id/photos/:photo_id", h.HTTP)
	g.GET("/buyback/:id/items/:item_id/devices", h.HTTP)
}
func adminFailure(c *gin.Context, e error) {
	status, code := 503, "BUYBACK_REQUEST_FAILED"
	switch {
	case errors.Is(e, authorization.ErrDenied):
		status, code = 403, "BUYBACK_DENIED"
	case errors.Is(e, ErrInput):
		status, code = 400, "BUYBACK_INVALID_INPUT"
	case errors.Is(e, ErrMissing):
		status, code = 404, "BUYBACK_NOT_FOUND"
	case errors.Is(e, ErrPhotoMissing):
		status, code = 404, "BUYBACK_PHOTO_NOT_FOUND"
	case errors.Is(e, ErrCommandState):
		status, code = 409, "BUYBACK_INVALID_STATE"
	case errors.Is(e, ErrCommandPhotos):
		status, code = 409, "BUYBACK_PHOTOS_REQUIRED"
	case errors.Is(e, ErrReviewNote):
		status, code = 409, "BUYBACK_REVIEW_NOTE_REQUIRED"
	case errors.Is(e, ErrDeviceMismatch):
		status, code = 409, "BUYBACK_DEVICE_MISMATCH"
	case errors.Is(e, ErrLinkDevices):
		status, code = 409, "BUYBACK_LINK_DEVICES_FIRST"
	}
	platform.Fail(c, status, code, "Buyback request unavailable")
}
func (h AdminHandler) HTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.required", "Sign in required")
		return
	}
	query, e := url.ParseQuery(c.Request.URL.RawQuery)
	if e != nil || c.Request.URL.ForceQuery {
		adminFailure(c, ErrInput)
		return
	}
	id, photo := c.Param("id"), c.Param("photo_id")
	if id != "" && len(query) > 0 {
		adminFailure(c, ErrInput)
		return
	}
	status, search, offset := "", "", 0
	for k, v := range query {
		if len(v) != 1 {
			adminFailure(c, ErrInput)
			return
		}
		switch k {
		case "status":
			status = v[0]
			switch status {
			case "awaiting_photos", "photo_review", "ready_to_ship", "in_transit", "received", "final_offer", "accepted", "paid", "return_requested", "returning", "returned", "canceled":
			default:
				adminFailure(c, ErrInput)
				return
			}
		case "q":
			var err error
			search, err = trimSubmission(v[0], 0, 100)
			if err != nil {
				adminFailure(c, err)
				return
			}
		case "offset":
			var err error
			offset, err = strconv.Atoi(v[0])
			if err != nil || offset < 0 || offset > 100000 || strconv.Itoa(offset) != v[0] {
				adminFailure(c, ErrInput)
				return
			}
		default:
			adminFailure(c, ErrInput)
			return
		}
	}
	var in *AdminCommand
	var inspection *InspectionCommand
	if c.Request.Method == "POST" {
		if strings.Split(c.GetHeader("Content-Type"), ";")[0] != "application/json" {
			adminFailure(c, ErrInput)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 65537))
		if err != nil {
			adminFailure(c, ErrInput)
			return
		}
		command, err := ParseAdminCommand(raw)
		if err == nil {
			in = &command
		} else {
			value, e := ParseInspectionCommand(raw)
			if e != nil {
				adminFailure(c, e)
				return
			}
			inspection = &value
		}
	}
	if h.DB == nil {
		adminFailure(c, ErrUnavailable)
		return
	}
	var out any
	e = h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if itemID := c.Param("item_id"); itemID != "" {
			v, err := InspectionCandidatesTx(c.Request.Context(), tx, actor, h.StoreID, id, itemID)
			out = v
			return err
		}
		if inspection != nil {
			v, err := InspectionTx(c.Request.Context(), tx, actor, h.StoreID, id, *inspection)
			out = v
			return err
		}
		if id == "" {
			v, err := AdminListTx(c.Request.Context(), tx, actor, h.StoreID, status, search, offset)
			out = v
			return err
		}
		if photo != "" {
			v, err := AdminPhotoTx(c.Request.Context(), tx, actor, h.StoreID, id, photo)
			out = v
			return err
		}
		v, err := AdminRequestTx(c.Request.Context(), tx, actor, h.StoreID, id, in)
		out = v
		return err
	})
	if e != nil {
		adminFailure(c, e)
		return
	}
	c.JSON(200, out)
}
