package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxImageBytes = 3 << 20
const MaxImageBodyBytes = (4 << 20) + 65536

var ErrImageInUse = errors.New("catalog image used by SKU")

type ImageEntry struct {
	ID  *string `json:"id"`
	URL string  `json:"url"`
}
type ImageUpload struct {
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Content  string `json:"content"`
}
type ImageEditInput struct {
	OperationID      string       `json:"operation_id"`
	ExpectedRevision string       `json:"expected_revision"`
	Images           []ImageEntry `json:"images"`
	ThumbnailIndex   *int         `json:"thumbnail_index"`
	Upload           *ImageUpload `json:"upload"`
}
type ImageItem struct {
	AdminImage
	VariantUses int `json:"variant_uses"`
}
type ImageContext struct {
	ProductID string      `json:"product_id"`
	Enabled   bool        `json:"enabled"`
	Revision  string      `json:"revision"`
	Thumbnail *string     `json:"thumbnail"`
	Images    []ImageItem `json:"images"`
}
type ImageResult struct {
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}

func imageDigest(v any) string {
	raw, _ := json.Marshal(v)
	s := sha256.Sum256(raw)
	return hex.EncodeToString(s[:])
}
func validExternalImage(s string) bool {
	u, e := url.Parse(s)
	return e == nil && utf8.ValidString(s) && len(s) <= 2048 && !containsControl(s) && u.Host != "" && u.User == nil && (u.Scheme == "https" || u.Scheme == "http")
}
func imageBytes(in *ImageUpload) ([]byte, error) {
	if in == nil {
		return nil, nil
	}
	if !createText(in.Filename, 200) || strings.ContainsAny(in.Filename, "/\\") || len(in.Content) > 4<<20 {
		return nil, errEditInput
	}
	data, e := base64.StdEncoding.Strict().DecodeString(in.Content)
	if e != nil || len(data) == 0 || len(data) > MaxImageBytes || base64.StdEncoding.EncodeToString(data) != in.Content {
		return nil, errEditInput
	}
	config, format, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 40000000 {
		return nil, errEditInput
	}
	if map[string]string{"jpeg": "image/jpeg", "png": "image/png", "webp": "image/webp"}[format] != in.MimeType {
		return nil, errEditInput
	}
	// Decode within the dimension budget, so a valid header cannot hide a
	// truncated/corrupt image that fails immediately in the storefront gallery.
	if _, _, e = image.Decode(bytes.NewReader(data)); e != nil {
		return nil, errEditInput
	}
	return data, nil
}
func validateImages(in ImageEditInput) ([]byte, error) {
	id, e := uuid.Parse(in.OperationID)
	rev, re := hex.DecodeString(in.ExpectedRevision)
	if e != nil || id.Version() != 4 || id.String() != in.OperationID || re != nil || len(rev) != 32 || hex.EncodeToString(rev) != in.ExpectedRevision || in.Images == nil || len(in.Images) > 1000 {
		return nil, errEditInput
	}
	seen := map[string]bool{}
	urls := map[string]bool{}
	for _, v := range in.Images {
		if v.ID != nil {
			if !validProductID(*v.ID) || seen[*v.ID] {
				return nil, errEditInput
			}
			seen[*v.ID] = true
		} else if !validExternalImage(v.URL) {
			return nil, errEditInput
		}
		if v.URL == "" || urls[v.URL] {
			return nil, errEditInput
		}
		urls[v.URL] = true
	}
	size := len(in.Images)
	if in.Upload != nil {
		size++
	}
	if size > 1000 || in.ThumbnailIndex != nil && (*in.ThumbnailIndex < -1 || *in.ThumbnailIndex >= size) {
		return nil, errEditInput
	}
	return imageBytes(in.Upload)
}

// Explicit recursive object check prevents duplicate/unknown keys before hashing.
func imageObject(raw []byte, keys ...string) (map[string]json.RawMessage, error) {
	allowed := map[string]bool{}
	for _, k := range keys {
		allowed[k] = true
	}
	out := map[string]json.RawMessage{}
	d := json.NewDecoder(bytes.NewReader(raw))
	start, e := d.Token()
	if e != nil || start != json.Delim('{') {
		return nil, errEditInput
	}
	for d.More() {
		tok, e := d.Token()
		k, ok := tok.(string)
		if e != nil || !ok || !allowed[k] || out[k] != nil {
			return nil, errEditInput
		}
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return nil, errEditInput
		}
		out[k] = v
	}
	end, e := d.Token()
	if e != nil || end != json.Delim('}') || len(out) != len(keys) || d.Decode(new(any)) != io.EOF {
		return nil, errEditInput
	}
	return out, nil
}
func decodeImages(r io.Reader) (ImageEditInput, error) {
	var in ImageEditInput
	raw, e := io.ReadAll(io.LimitReader(r, MaxImageBodyBytes+1))
	if e != nil {
		return in, e
	}
	if len(raw) > MaxImageBodyBytes || !utf8.Valid(raw) {
		return in, errEditInput
	}
	obj, e := imageObject(raw, "operation_id", "expected_revision", "images", "thumbnail_index", "upload")
	if e != nil {
		return in, e
	}
	var entries []json.RawMessage
	if json.Unmarshal(obj["images"], &entries) != nil {
		return in, errEditInput
	}
	for _, v := range entries {
		if _, e := imageObject(v, "id", "url"); e != nil {
			return in, e
		}
	}
	if !bytes.Equal(bytes.TrimSpace(obj["upload"]), []byte("null")) {
		if _, e := imageObject(obj["upload"], "filename", "mime_type", "content"); e != nil {
			return in, e
		}
	}
	if json.Unmarshal(raw, &in) != nil {
		return in, errEditInput
	}
	if _, e = validateImages(in); e != nil {
		return in, e
	}
	return in, nil
}
func lockImages(tx *gorm.DB, p string) (bool, error) {
	var cap struct {
		Version int
		Enabled bool
	}
	if tx.Table("catalog_command_capabilities").Where("command='product.images'").Clauses(clause.Locking{Strength: "SHARE"}).Take(&cap).Error != nil || cap.Version != 1 {
		return false, ErrSchema
	}
	var product struct{ ID string }
	if e := tx.Table("catalog_products").Select("id").Where("id=? AND deleted_at IS NULL", p).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&product).Error; e != nil {
		return false, e
	}
	return cap.Enabled, nil
}
func loadImages(tx *gorm.DB, p string, enabled bool) (ImageContext, error) {
	out := ImageContext{ProductID: p, Enabled: enabled, Images: []ImageItem{}}
	var product struct{ Thumbnail *string }
	if e := tx.Table("catalog_products").Select("thumbnail").Where("id=? AND deleted_at IS NULL", p).Take(&product).Error; e != nil {
		return out, e
	}
	out.Thumbnail = product.Thumbnail
	if tx.Table("catalog_images AS i").Select("i.id,i.url,i.rank,(SELECT COUNT(*) FROM catalog_variants v WHERE v.deleted_at IS NULL AND ((v.product_id=i.product_id AND v.thumbnail=i.url) OR EXISTS(SELECT 1 FROM catalog_variant_images vi WHERE vi.variant_id=v.id AND vi.image_id=i.id AND vi.deleted_at IS NULL))) AS variant_uses").Where("i.product_id=? AND i.deleted_at IS NULL", p).Order("i.rank,i.id").Limit(1001).Scan(&out.Images).Error != nil || len(out.Images) > 1000 {
		return out, ErrSchema
	}
	out.Revision = imageDigest(struct {
		Thumbnail *string
		Images    []ImageItem
	}{out.Thumbnail, out.Images})
	return out, nil
}
func imageAccess(ctx context.Context, tx *gorm.DB, actor identity.User) error {
	if e := authorization.CheckTx(ctx, tx, actor, authorization.ProductRead, authorization.ProductWrite); e != nil {
		return e
	}
	return Ready(ctx, tx)
}
func (h Handler) ReadImages(ctx context.Context, p string, actor identity.User) (ImageContext, error) {
	var out ImageContext
	if h.DB == nil || !validProductID(p) {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := imageAccess(ctx, tx, actor); e != nil {
			return e
		}
		enabled, e := lockImages(tx, p)
		if e != nil {
			return e
		}
		out, e = loadImages(tx, p, enabled)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}
func (h Handler) EditImages(ctx context.Context, p string, actor identity.User, in ImageEditInput) (ImageResult, error) {
	var out ImageResult
	data, e := validateImages(in)
	if e != nil || !validProductID(p) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	requestSHA := imageDigest(in)
	e = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := imageAccess(ctx, tx, actor); e != nil {
			return e
		}
		enabled, e := lockImages(tx, p)
		if e != nil {
			return e
		}
		var old struct{ ProductID, ActorID, RequestSHA256, ResultRevision string }
		e = tx.Table("catalog_image_edits").Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&old).Error
		if e == nil {
			if old.ProductID != p || old.ActorID != actor.ID || old.RequestSHA256 != requestSHA {
				return ErrEditConflict
			}
			out = ImageResult{old.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !enabled {
			return ErrSchema
		}
		before, e := loadImages(tx, p, enabled)
		if e != nil {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		current := map[string]ImageItem{}
		for _, v := range before.Images {
			current[v.ID] = v
		}
		keep := map[string]bool{}
		for _, v := range in.Images {
			if v.ID != nil {
				row, ok := current[*v.ID]
				if !ok || row.URL != v.URL {
					return errEditInput
				}
				keep[*v.ID] = true
			}
		}
		for _, v := range before.Images {
			if !keep[v.ID] && v.VariantUses > 0 {
				return ErrImageInUse
			}
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		images := []AdminImage{}
		for _, v := range before.Images {
			if !keep[v.ID] {
				if e := tx.Table("catalog_images").Where("id=?", v.ID).Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; e != nil {
					return e
				}
			}
		}
		for rank, v := range in.Images {
			id := "img_" + uuid.NewString()
			if v.ID != nil {
				id = *v.ID
				if e := tx.Table("catalog_images").Where("id=?", id).Updates(map[string]any{"rank": rank, "updated_at": now}).Error; e != nil {
					return e
				}
			} else {
				if e := tx.Table("catalog_images").Create(map[string]any{"id": id, "url": v.URL, "rank": rank, "product_id": p, "created_at": now, "updated_at": now}).Error; e != nil {
					return e
				}
			}
			images = append(images, AdminImage{id, v.URL, rank})
		}
		if in.Upload != nil {
			id := "img_" + uuid.NewString()
			u := "/api/store/catalog-images/" + id
			rank := len(images)
			if e := tx.Table("catalog_images").Create(map[string]any{"id": id, "url": u, "rank": rank, "product_id": p, "created_at": now, "updated_at": now}).Error; e != nil {
				return e
			}
			digest := sha256.Sum256(data)
			if e := tx.Table("catalog_image_assets").Create(map[string]any{"image_id": id, "filename": in.Upload.Filename, "mime_type": in.Upload.MimeType, "sha256": hex.EncodeToString(digest[:]), "content": data, "created_at": now}).Error; e != nil {
				return e
			}
			images = append(images, AdminImage{id, u, rank})
		}
		var thumbnail *string
		if in.ThumbnailIndex != nil {
			if *in.ThumbnailIndex == -1 {
				thumbnail = before.Thumbnail
				for _, old := range before.Images {
					if before.Thumbnail != nil && old.URL == *before.Thumbnail {
						return errEditInput
					}
				}
			} else {
				thumbnail = &images[*in.ThumbnailIndex].URL
			}
		}
		if e := tx.Table("catalog_products").Where("id=?", p).Updates(map[string]any{"thumbnail": thumbnail, "updated_at": now}).Error; e != nil {
			return e
		}
		after, e := loadImages(tx, p, enabled)
		if e != nil {
			return e
		}
		beforeJSON, _ := json.Marshal(before)
		afterJSON, _ := json.Marshal(after)
		out = ImageResult{after.Revision, false}
		return tx.Table("catalog_image_edits").Create(map[string]any{"operation_id": in.OperationID, "product_id": p, "actor_id": actor.ID, "request_sha256": requestSHA, "before_json": string(beforeJSON), "after_json": string(afterJSON), "result_revision": after.Revision, "created_at": now}).Error
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}
