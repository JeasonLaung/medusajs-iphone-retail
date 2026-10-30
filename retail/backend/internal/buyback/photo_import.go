package buyback

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrPhotoImport = errors.New("private photo import evidence mismatch")
var localPhotoKey = regexp.MustCompile(`^private-[0-9]+-buyback-[0-9a-fA-F-]+\.(jpg|png|webp)$`)

// ReadLegacyPhoto reads only a single original local-provider basename through
// an OS-constrained root. It never follows a path outside that root or writes
// a source file. A manifest must resolve the Medusa storage ID to this key.
func ReadLegacyPhoto(root *os.Root, key string) ([]byte, error) {
	if root == nil || !localPhotoKey.MatchString(key) {
		return nil, ErrPhotoImport
	}
	f, err := root.Open(key)
	if err != nil {
		return nil, ErrPhotoImport
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 3 || info.Size() > MaxPhotoBytes {
		return nil, ErrPhotoImport
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxPhotoBytes+1))
	if err != nil || len(b) > MaxPhotoBytes || photoMIME(b) == "" {
		return nil, ErrPhotoImport
	}
	return b, nil
}

// ImportLegacyPhotoTx requires a complete accepted source snapshot and an
// isolated caller-owned transaction. Roll back on any error. Original photo
// fields remain unchanged, so metadata import replay still proves equality.
// No account binding, source deletion, notification or commit is performed.
func ImportLegacyPhotoTx(ctx context.Context, tx *gorm.DB, snapshot, requestID, photoID, sourceKey, expectedSHA string, data []byte) error {
	snapshotBytes, e := hex.DecodeString(snapshot)
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if tx == nil || len(snapshotBytes) != 32 || e != nil || strings.ToLower(snapshot) != snapshot || !validID(requestID, 191) || !validID(photoID, 191) || sourceKey == "" || strings.HasPrefix(sourceKey, "buyback-db:") || expectedSHA != digest || len(data) > MaxPhotoBytes || photoMIME(data) == "" {
		return ErrPhotoImport
	}
	tx = tx.WithContext(ctx)
	var request struct{ ID string }
	if tx.Table("buyback_requests").Select("id").Where("id=?", requestID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&request).Error != nil {
		return ErrPhotoImport
	}
	var photo struct{ ID, StorageKey, MimeType, ReviewStatus string }
	if tx.Table("buyback_photos").Select("id,storage_key,mime_type,review_status").Where("id=? AND request_id=?", photoID, requestID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&photo).Error != nil || photo.StorageKey != sourceKey || photo.MimeType != photoMIME(data) || photo.ReviewStatus == "expired" {
		return ErrPhotoImport
	}
	var old struct {
		SourceKey, ContentSHA256, MimeType string
		SnapshotSHA256                     []byte
	}
	err := tx.Table("buyback_photo_imports").Where("photo_id=?", photoID).Take(&old).Error
	if err == nil {
		if old.SourceKey != sourceKey || old.ContentSHA256 != digest || old.MimeType != photo.MimeType || !bytes.Equal(old.SnapshotSHA256, snapshotBytes) {
			return ErrPhotoImport
		}
		var saved struct {
			Content          []byte
			SHA256, MimeType string
		}
		if tx.Table("buyback_photo_bytes").Where("photo_id=?", photoID).Take(&saved).Error != nil || saved.SHA256 != digest || saved.MimeType != photo.MimeType || !bytes.Equal(saved.Content, data) {
			return ErrPhotoImport
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrPhotoImport
	}
	// An imported metadata row must already carry exact source provenance.
	var accepted int64
	if tx.Table("migration_catalog_rows").Where("source_table=? AND snapshot_sha256=?", []byte("buyback_photo"), snapshotBytes).Where("full_identity=?", photoIdentity(photoID)).Count(&accepted).Error != nil || accepted != 1 {
		return ErrPhotoImport
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if tx.Table("buyback_photo_bytes").Create(map[string]any{"photo_id": photoID, "mime_type": photo.MimeType, "sha256": digest, "content": data, "created_at": now}).Error != nil {
		return ErrPhotoImport
	}
	if tx.Table("buyback_photo_imports").Create(map[string]any{"photo_id": photoID, "source_key": sourceKey, "snapshot_sha256": snapshotBytes, "content_sha256": digest, "mime_type": photo.MimeType, "created_at": now}).Error != nil {
		return ErrPhotoImport
	}
	return nil
}

func photoIdentity(id string) []byte {
	b := []byte{1}
	// Catalog identity framing: version byte, uint64 length, exact UTF-8 ID.
	for shift := 56; shift >= 0; shift -= 8 {
		b = append(b, byte(uint64(len(id))>>shift))
	}
	return append(b, []byte(id)...)
}

func verifiedImportedPhoto(tx *gorm.DB, id, key, mime string) (string, error) {
	var row struct{ SourceKey, ContentSHA256, MimeType string }
	if tx.Table("buyback_photo_imports").Where("photo_id=?", id).Take(&row).Error != nil || row.SourceKey != key || row.MimeType != mime {
		return "", ErrPhotoMissing
	}
	return row.ContentSHA256, nil
}
