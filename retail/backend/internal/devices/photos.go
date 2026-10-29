package devices

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const MaxPhotoBytes = 3 << 20

type PhotoInput struct {
	OperationID string `json:"operation_id"`
	Filename    string `json:"filename"`
	MimeType    string `json:"mime_type"`
	Content     string `json:"content"`
}
type photoRow struct {
	ID, OperationID, ActorID, Filename, MimeType, SHA256 string
	Content                                              []byte
}

func photoMIME(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{255, 216, 255}):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte{137, 80, 78, 71, 13, 10, 26, 10}):
		return "image/png"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}
func photoURL(id string) string { return "/api/store/device-photos/" + id }
func validPhotoRef(p Photo) bool {
	if len(p.ID) != 43 || p.ID[:7] != "dphoto_" || p.URL != photoURL(p.ID) {
		return false
	}
	id, e := uuid.Parse(p.ID[7:])
	return e == nil && id.Version() == 4 && id.String() == p.ID[7:]
}
func verifyPhoto(row photoRow) bool {
	sum := sha256.Sum256(row.Content)
	return len(row.Content) <= MaxPhotoBytes && photoMIME(row.Content) != "" && photoMIME(row.Content) == row.MimeType && hex.EncodeToString(sum[:]) == row.SHA256
}
func (h Handler) SavePhoto(ctx context.Context, actor identity.User, in PhotoInput) (Photo, bool, error) {
	var out Photo
	id, e := uuid.Parse(in.OperationID)
	if e != nil || id.Version() != 4 || id.String() != in.OperationID || in.Filename == "" || !intakeText(in.Filename, 200) || len(in.Content) > 4194304 {
		return out, false, ErrInput
	}
	data, e := base64.StdEncoding.Strict().DecodeString(in.Content)
	if e != nil || base64.StdEncoding.EncodeToString(data) != in.Content || len(data) > MaxPhotoBytes || photoMIME(data) == "" || photoMIME(data) != in.MimeType {
		return out, false, ErrInput
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if h.DB == nil {
		return out, false, ErrUnavailable
	}
	replay := false
	e = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := intakeAuthority(ctx, tx, actor, true); e != nil {
			return e
		}
		if _, e := sourceGate(ctx, tx); e != nil {
			return e
		}
		var prior photoRow
		e := tx.Table("device_photos").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ActorID != actor.ID || prior.Filename != in.Filename || prior.MimeType != in.MimeType || prior.SHA256 != digest || !bytes.Equal(prior.Content, data) {
				return ErrConflict
			}
			if !verifyPhoto(prior) {
				return ErrUnavailable
			}
			out = Photo{prior.ID, photoURL(prior.ID)}
			replay = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		out = Photo{"dphoto_" + uuid.NewString(), ""}
		out.URL = photoURL(out.ID)
		if tx.Exec("INSERT INTO device_photos(id,operation_id,actor_id,filename,mime_type,sha256,content,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6))", out.ID, in.OperationID, actor.ID, in.Filename, in.MimeType, digest, data).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, replay, e
}
func verifyPhotosTx(tx *gorm.DB, photos []Photo) error {
	for _, p := range photos {
		var row photoRow
		e := tx.Table("device_photos").Where("id=?", p.ID).Take(&row).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrInput
		}
		if e != nil || !verifyPhoto(row) {
			return ErrUnavailable
		}
	}
	return nil
}
func decodePhoto(r io.Reader) (PhotoInput, error) {
	var in PhotoInput
	raw, e := io.ReadAll(io.LimitReader(r, platform.MaxDevicePhotoBodyBytes+1))
	if e != nil || int64(len(raw)) > platform.MaxDevicePhotoBodyBytes || !utf8.Valid(raw) {
		return in, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if uniqueJSON(d) != nil || d.Decode(new(any)) != io.EOF {
		return in, ErrInput
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		return in, ErrInput
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 4 {
		return in, ErrInput
	}
	for _, v := range fields {
		if len(v) == 0 || v[0] != '"' {
			return in, ErrInput
		}
	}
	return in, nil
}
func (h Handler) PhotoUploadHTTP(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		trackingFail(c, ErrInput)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "device.content_type", "JSON required")
		return
	}
	actor, ok := c.MustGet("admin_user").(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	in, e := decodePhoto(c.Request.Body)
	if e != nil {
		trackingFail(c, e)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	photo, replay, e := h.SavePhoto(ctx, actor, in)
	if e != nil {
		trackingFail(c, e)
		return
	}
	c.JSON(200, gin.H{"photo": photo, "replayed": replay})
}
func (h Handler) PhotoHTTP(c *gin.Context) {
	id := c.Param("photo_id")
	if c.Request.URL.RawQuery != "" || !validPhotoRef(Photo{id, photoURL(id)}) {
		trackingFail(c, ErrInput)
		return
	}
	if h.DB == nil {
		trackingFail(c, ErrUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	var row photoRow
	e := h.DB.WithContext(ctx).Table("device_photos").Where("id=?", id).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		trackingFail(c, ErrNotFound)
		return
	}
	if e != nil || !verifyPhoto(row) {
		trackingFail(c, ErrUnavailable)
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("Content-Disposition", "inline")
	c.Header("ETag", `"`+row.SHA256+`"`)
	c.Data(200, row.MimeType, row.Content)
}
