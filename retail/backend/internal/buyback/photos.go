package buyback

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxPhotoBytes = 3 << 20
const MaxPhotoBodyBytes = (4 << 20) + 8192

var ErrPhotoMissing = errors.New("buyback photo not found")
var ErrPhotoState = errors.New("buyback photo operation unavailable in this state")
var ErrPhotoLimit = errors.New("buyback photo limit reached")
var photoContent = regexp.MustCompile(`^[A-Za-z0-9+/]+={0,2}$`)
var photoTypes = map[string]bool{"front": true, "back": true, "edges": true, "damage": true, "packaging": true, "sealed": true}

type PhotoUpload struct {
	ItemID    string `json:"item_id"`
	PhotoType string `json:"photo_type"`
	MimeType  string `json:"mime_type"`
	Content   string `json:"content"`
}
type PhotoContent struct {
	Content  string `json:"content"`
	MimeType string `json:"mime_type"`
}

func photoMIME(b []byte) string {
	if bytes.HasPrefix(b, []byte{255, 216, 255}) {
		return "image/jpeg"
	}
	if bytes.HasPrefix(b, []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return "image/png"
	}
	if len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return "image/webp"
	}
	return ""
}
func photoData(in PhotoUpload) ([]byte, error) {
	if _, e := trimSubmission(in.ItemID, 1, 100); e != nil {
		return nil, ErrInput
	}
	if !photoTypes[in.PhotoType] || len(in.Content) < 4 || len(in.Content) > 4200000 || !photoContent.MatchString(in.Content) {
		return nil, ErrInput
	}
	// Preserve Buffer.from(base64) semantics for the original validator's alphabet:
	// padding is optional and an incomplete final sextet is ignored.
	text := strings.TrimRight(in.Content, "=")
	if len(text)%4 == 1 {
		text = text[:len(text)-1]
	}
	data, e := base64.RawStdEncoding.DecodeString(text)
	if e != nil || len(data) > MaxPhotoBytes || photoMIME(data) == "" || photoMIME(data) != in.MimeType {
		return nil, ErrInput
	}
	return data, nil
}
func ParsePhotoUpload(raw []byte) (PhotoUpload, error) {
	var in PhotoUpload
	if decodeBounded(raw, &in, MaxPhotoBodyBytes, "item_id", "photo_type", "mime_type", "content") != nil {
		return in, ErrInput
	}
	var e error
	in.ItemID, e = trimSubmission(in.ItemID, 1, 100)
	if e != nil {
		return PhotoUpload{}, e
	}
	if _, e := photoData(in); e != nil {
		return PhotoUpload{}, e
	}
	return in, nil
}

// StagePhotoUpload requires the caller's freshly authenticated account transaction.
// Locking the application serializes the original per-item quota and state gate.
func StagePhotoUpload(ctx context.Context, tx *gorm.DB, storeID, accountID, requestID string, in PhotoUpload) (PublicPhoto, error) {
	var out PublicPhoto
	var e error
	in.ItemID, e = trimSubmission(in.ItemID, 1, 100)
	if e != nil {
		return out, e
	}
	data, e := photoData(in)
	if e != nil {
		return out, e
	}
	if tx == nil || !validID(requestID, 191) {
		return out, ErrMissing
	}
	tx = tx.WithContext(ctx)
	var locked struct{ ID string }
	e = tx.Table("buyback_requests r").Select("r.id").Joins("JOIN customer_buyback_submissions b ON b.request_id=r.id AND b.customer_id=r.customer_id").Where("r.id=? AND r.deleted_at IS NULL AND b.customer_id=? AND b.store_id=?", requestID, accountID, storeID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&locked).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return out, ErrMissing
	}
	if e != nil {
		return out, ErrUnavailable
	}
	request, e := ReadRequest(ctx, tx, storeID, accountID, requestID)
	if e != nil {
		return out, e
	}
	var gate struct {
		Version int
		Enabled bool
	}
	if tx.Table("buyback_request_runtime").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&gate).Error != nil || gate.Version != 1 || !gate.Enabled {
		return out, ErrUnavailable
	}
	if request.Status != "awaiting_photos" && request.Status != "ready_to_ship" || request.Status == "ready_to_ship" && in.PhotoType != "packaging" && in.PhotoType != "sealed" {
		return out, ErrPhotoState
	}
	found := false
	for _, item := range request.Items {
		if item.ID == in.ItemID {
			found = true
		}
	}
	if !found {
		return out, ErrPhotoMissing
	}
	var count int64
	if tx.Table("buyback_photos").Where("request_id=? AND item_id=? AND deleted_at IS NULL", requestID, in.ItemID).Count(&count).Error != nil {
		return out, ErrUnavailable
	}
	if count >= 30 {
		return out, ErrPhotoLimit
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	out = PublicPhoto{ID: "bbp_" + uuid.NewString(), RequestID: requestID, ItemID: in.ItemID, PhotoType: in.PhotoType, UploadedAt: now, ReviewStatus: "pending", ReviewNote: "", MimeType: in.MimeType}
	if tx.Table("buyback_photos").Create(map[string]any{"id": out.ID, "request_id": requestID, "item_id": in.ItemID, "photo_type": in.PhotoType, "uploaded_at": now, "storage_key": "buyback-db:" + out.ID, "mime_type": in.MimeType, "review_status": "pending", "review_note": "", "created_at": now, "updated_at": now}).Error != nil {
		return PublicPhoto{}, ErrUnavailable
	}
	sum := sha256.Sum256(data)
	if tx.Table("buyback_photo_bytes").Create(map[string]any{"photo_id": out.ID, "mime_type": in.MimeType, "sha256": hex.EncodeToString(sum[:]), "content": data, "created_at": now}).Error != nil {
		return PublicPhoto{}, ErrUnavailable
	}
	return out, nil
}

func ReadPhoto(ctx context.Context, tx *gorm.DB, storeID, accountID, requestID, photoID string) (PhotoContent, error) {
	var out PhotoContent
	if _, e := ReadRequest(ctx, tx, storeID, accountID, requestID); e != nil {
		return out, e
	}
	return readPhotoBytes(ctx, tx, requestID, photoID)
}
func readPhotoBytes(ctx context.Context, tx *gorm.DB, requestID, photoID string) (PhotoContent, error) {
	var out PhotoContent
	if !validID(photoID, 191) {
		return out, ErrPhotoMissing
	}
	var row struct{ ID, StorageKey, MimeType string }
	e := tx.WithContext(ctx).Table("buyback_photos").Select("id,storage_key,mime_type").Where("id=? AND request_id=? AND deleted_at IS NULL AND review_status<>'expired'", photoID, requestID).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return out, ErrPhotoMissing
	}
	if e != nil {
		return out, ErrUnavailable
	}
	importedDigest := ""
	if row.StorageKey != "buyback-db:"+photoID {
		importedDigest, e = verifiedImportedPhoto(tx, photoID, row.StorageKey, row.MimeType)
		if e != nil {
			return out, e
		}
	}
	var saved struct {
		MimeType, SHA256 string
		Content          []byte
	}
	if tx.Table("buyback_photo_bytes").Select("mime_type,sha256,content").Where("photo_id=?", photoID).Take(&saved).Error != nil {
		return out, ErrUnavailable
	}
	sum := sha256.Sum256(saved.Content)
	if importedDigest != "" && importedDigest != hex.EncodeToString(sum[:]) {
		return out, ErrUnavailable
	}
	if len(saved.Content) > MaxPhotoBytes || photoMIME(saved.Content) == "" || photoMIME(saved.Content) != row.MimeType || saved.MimeType != row.MimeType || saved.SHA256 != hex.EncodeToString(sum[:]) {
		return out, ErrUnavailable
	}
	return PhotoContent{base64.StdEncoding.EncodeToString(saved.Content), row.MimeType}, nil
}
