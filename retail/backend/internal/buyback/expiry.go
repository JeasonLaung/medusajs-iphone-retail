package buyback

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const PhotoRetentionDays = 180

type ExpiryPage struct {
	Requests    int    `json:"requests"`
	Photos      int    `json:"photos"`
	Expired     int    `json:"expired"`
	LegacyFiles int    `json:"legacy_files"`
	Next        string `json:"next,omitempty"`
}

// ExpirePhotoPage preserves the original closed_at < now-180days rule,
// independently of customer identity. Each request is its own transaction.
// Keyset paging also advances past legacy objects pending a verified importer.
// With apply=false it only counts candidates and never locks or deletes data.
func ExpirePhotoPage(ctx context.Context, db *gorm.DB, now time.Time, after string, limit int, apply bool) (ExpiryPage, error) {
	out := ExpiryPage{}
	if db == nil || now.IsZero() || limit < 1 || limit > 100 || after != "" && !validID(after, 191) {
		return out, ErrInput
	}
	cutoff := now.UTC().Truncate(time.Microsecond).Add(-PhotoRetentionDays * 24 * time.Hour)
	var requests []struct{ ID string }
	if db.WithContext(ctx).Table("buyback_requests").Select("id").Where("deleted_at IS NULL AND closed_at<? AND id>?", cutoff, after).Order("id").Limit(limit).Find(&requests).Error != nil {
		return out, ErrUnavailable
	}
	for _, request := range requests {
		page := ExpiryPage{}
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var row struct{ ID string }
			q := tx.Table("buyback_requests").Select("id").Where("id=? AND deleted_at IS NULL AND closed_at<?", request.ID, cutoff)
			if apply {
				q = q.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			e := q.Take(&row).Error
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return nil
			}
			if e != nil {
				return ErrUnavailable
			}
			page.Requests = 1
			var photos []struct{ ID, StorageKey string }
			q = tx.Table("buyback_photos").Select("id,storage_key").Where("request_id=? AND deleted_at IS NULL AND review_status<>'expired'", row.ID).Order("id")
			if apply {
				q = q.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			// Original workflow permits unlimited historical photos; page without
			// loading image bytes or imposing the new upload quota on old records.
			last := ""
			for {
				photos = nil
				if q.Where("id>?", last).Limit(100).Find(&photos).Error != nil {
					return ErrUnavailable
				}
				if len(photos) == 0 {
					break
				}
				for _, photo := range photos {
					page.Photos++
					last = photo.ID
					if photo.StorageKey != "" && photo.StorageKey != "buyback-db:"+photo.ID {
						page.LegacyFiles++
						continue
					}
					if !apply {
						continue
					}
					// Metadata and blob are in one MySQL transaction. A later error
					// rolls back both, unlike an irreversible external file delete.
					if tx.Table("buyback_photo_bytes").Where("photo_id=?", photo.ID).Delete(&struct{}{}).Error != nil {
						return ErrUnavailable
					}
					if tx.Table("buyback_photos").Where("id=? AND request_id=? AND deleted_at IS NULL", photo.ID, row.ID).Updates(map[string]any{"storage_key": "", "review_status": "expired", "updated_at": now.UTC().Truncate(time.Microsecond)}).Error != nil {
						return ErrUnavailable
					}
					page.Expired++
				}
			}
			return nil
		})
		if err != nil {
			return out, err
		}
		out.Requests += page.Requests
		out.Photos += page.Photos
		out.Expired += page.Expired
		out.LegacyFiles += page.LegacyFiles
		out.Next = request.ID
	}
	return out, nil
}
