package catalog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type VariantImageInput struct {
	OperationID      string   `json:"operation_id"`
	ExpectedRevision string   `json:"expected_revision"`
	ImageIDs         []string `json:"image_ids"`
	ThumbnailImageID *string  `json:"thumbnail_image_id"`
	KeepThumbnail    bool     `json:"keep_thumbnail"`
}
type VariantImageContext struct {
	ProductID   string       `json:"product_id"`
	VariantID   string       `json:"variant_id"`
	Enabled     bool         `json:"enabled"`
	Revision    string       `json:"revision"`
	Thumbnail   *string      `json:"thumbnail"`
	Images      []AdminImage `json:"images"`
	SelectedIDs []string     `json:"selected_ids"`
}

func validVariantImages(in VariantImageInput) bool {
	id, e := uuid.Parse(in.OperationID)
	rev, re := hex.DecodeString(in.ExpectedRevision)
	if e != nil || id.Version() != 4 || id.String() != in.OperationID || re != nil || len(rev) != 32 || hex.EncodeToString(rev) != in.ExpectedRevision || in.ImageIDs == nil || len(in.ImageIDs) > 1000 || in.KeepThumbnail && in.ThumbnailImageID != nil {
		return false
	}
	seen := map[string]bool{}
	for _, id := range in.ImageIDs {
		if !validProductID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return in.ThumbnailImageID == nil || seen[*in.ThumbnailImageID]
}
func decodeVariantImages(r io.Reader) (VariantImageInput, error) {
	var in VariantImageInput
	raw, e := io.ReadAll(io.LimitReader(r, 262145))
	if e != nil {
		return in, e
	}
	if len(raw) > 262144 || !utf8.Valid(raw) {
		return in, errEditInput
	}
	fields, e := imageObject(raw, "operation_id", "expected_revision", "image_ids", "thumbnail_image_id", "keep_thumbnail")
	if e != nil {
		return in, e
	}
	for key, value := range fields {
		if key != "thumbnail_image_id" && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return in, errEditInput
		}
	}
	if json.Unmarshal(raw, &in) != nil || !validVariantImages(in) {
		return in, errEditInput
	}
	return in, nil
}

// Caller holds the product lock shared with gallery editing and SKU creation.
func lockImageVariant(tx *gorm.DB, p, v string) error {
	var row struct{ ID string }
	return tx.Table("catalog_variants").Select("id").Where("id=? AND product_id=? AND deleted_at IS NULL", v, p).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
}
func loadVariantImages(tx *gorm.DB, p, v string, enabled bool) (VariantImageContext, error) {
	out := VariantImageContext{ProductID: p, VariantID: v, Enabled: enabled, Images: []AdminImage{}, SelectedIDs: []string{}}
	var variant struct{ Thumbnail *string }
	if e := tx.Table("catalog_variants").Select("thumbnail").Where("id=? AND product_id=? AND deleted_at IS NULL", v, p).Take(&variant).Error; e != nil {
		return out, e
	}
	out.Thumbnail = variant.Thumbnail
	if tx.Table("catalog_images").Select("id,url,`rank`").Where("product_id=? AND deleted_at IS NULL", p).Order("`rank`,id").Limit(1001).Scan(&out.Images).Error != nil || len(out.Images) > 1000 {
		return out, ErrSchema
	}
	var links []struct{ ID, ImageID string }
	if tx.Table("catalog_variant_images").Select("id,image_id").Where("variant_id=? AND deleted_at IS NULL", v).Order("image_id,id").Limit(1001).Scan(&links).Error != nil || len(links) > 1000 {
		return out, ErrSchema
	}
	allowed := map[string]bool{}
	for _, i := range out.Images {
		allowed[i.ID] = true
	}
	selected := map[string]bool{}
	for _, link := range links {
		if !allowed[link.ImageID] || selected[link.ImageID] {
			return out, ErrSchema
		}
		selected[link.ImageID] = true
		out.SelectedIDs = append(out.SelectedIDs, link.ImageID)
	}
	out.Revision = imageDigest(struct {
		VariantID string
		Thumbnail *string
		Images    []AdminImage
		Links     []struct{ ID, ImageID string }
	}{v, out.Thumbnail, out.Images, links})
	return out, nil
}
func (h Handler) ReadVariantImages(ctx context.Context, p, v string, actor identity.User) (VariantImageContext, error) {
	var out VariantImageContext
	if !validProductID(p) || !validProductID(v) {
		return out, errEditInput
	}
	if h.DB == nil {
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
		if e := lockImageVariant(tx, p, v); e != nil {
			return e
		}
		out, e = loadVariantImages(tx, p, v, enabled)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}
func (h Handler) EditVariantImages(ctx context.Context, p, v string, actor identity.User, in VariantImageInput) (ImageResult, error) {
	var out ImageResult
	if !validProductID(p) || !validProductID(v) || !validVariantImages(in) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	// Use the existing image receipt, with explicit command/variant domain binding.
	requestSHA := imageDigest(struct {
		Command, VariantID string
		Input              VariantImageInput
	}{"variant.images", v, in})
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := imageAccess(ctx, tx, actor); e != nil {
			return e
		}
		enabled, e := lockImages(tx, p)
		if e != nil {
			return e
		}
		if e := lockImageVariant(tx, p, v); e != nil {
			return e
		}
		var receipt struct{ ProductID, ActorID, RequestSHA256, ResultRevision string }
		e = tx.Table("catalog_image_edits").Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&receipt).Error
		if e == nil {
			if receipt.ProductID != p || receipt.ActorID != actor.ID || receipt.RequestSHA256 != requestSHA {
				return ErrEditConflict
			}
			out = ImageResult{receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !enabled {
			return ErrSchema
		}
		before, e := loadVariantImages(tx, p, v, enabled)
		if e != nil {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		images := map[string]AdminImage{}
		for _, i := range before.Images {
			images[i.ID] = i
		}
		selected := map[string]bool{}
		for _, id := range in.ImageIDs {
			if _, ok := images[id]; !ok {
				return errEditInput
			}
			selected[id] = true
		}
		var thumbnail *string
		if in.KeepThumbnail {
			thumbnail = before.Thumbnail
		} else if in.ThumbnailImageID != nil {
			image := images[*in.ThumbnailImageID]
			thumbnail = &image.URL
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		prior := map[string]bool{}
		for _, id := range before.SelectedIDs {
			prior[id] = true
			if !selected[id] {
				if e := tx.Table("catalog_variant_images").Where("variant_id=? AND image_id=? AND deleted_at IS NULL", v, id).Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; e != nil {
					return e
				}
			}
		}
		ids := append([]string{}, in.ImageIDs...)
		sort.Strings(ids)
		for _, id := range ids {
			if !prior[id] {
				if e := tx.Table("catalog_variant_images").Create(map[string]any{"id": "vimg_" + uuid.NewString(), "variant_id": v, "image_id": id, "created_at": now, "updated_at": now}).Error; e != nil {
					return e
				}
			}
		}
		if e := tx.Table("catalog_variants").Where("id=?", v).Updates(map[string]any{"thumbnail": thumbnail, "updated_at": now}).Error; e != nil {
			return e
		}
		after, e := loadVariantImages(tx, p, v, enabled)
		if e != nil {
			return e
		}
		oldJSON, _ := json.Marshal(before)
		newJSON, _ := json.Marshal(after)
		out = ImageResult{after.Revision, false}
		return tx.Table("catalog_image_edits").Create(map[string]any{"operation_id": in.OperationID, "product_id": p, "actor_id": actor.ID, "request_sha256": requestSHA, "before_json": string(oldJSON), "after_json": string(newJSON), "result_revision": after.Revision, "created_at": now}).Error
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}
