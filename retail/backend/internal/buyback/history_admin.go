package buyback

import (
	"context"
	"encoding/hex"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BindHistoricalStoreTx is an offline migration operation, never an HTTP
// command. The reviewed store is explicit; matching customer IDs/emails grants
// no scope. Source provenance must already be staged in the same transaction.
func BindHistoricalStoreTx(ctx context.Context, tx *gorm.DB, request, store, snapshot string) error {
	hash, e := hex.DecodeString(snapshot)
	if tx == nil || !validID(request, 191) || !validID(store, 191) || e != nil || len(hash) != 32 || strings.ToLower(snapshot) != snapshot {
		return ErrInput
	}
	tx = tx.WithContext(ctx)
	var row struct{ ID string }
	if tx.Table("buyback_requests").Select("id").Where("id=?", request).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
		return ErrMissing
	}
	var n int64
	if tx.Table("commerce_stores").Where("id=? AND deleted_at IS NULL", store).Count(&n).Error != nil || n != 1 {
		return ErrMissing
	}
	if tx.Table("customer_buyback_submissions").Where("request_id=?", request).Count(&n).Error != nil || n != 0 {
		return ErrInput
	}
	if tx.Table("migration_catalog_rows").Where("source_table=? AND full_identity=? AND snapshot_sha256=?", []byte("buyback_request"), photoIdentity(request), hash).Count(&n).Error != nil || n != 1 {
		return ErrInput
	}
	if tx.Exec(`INSERT INTO buyback_history_stores(request_id,store_id,snapshot_sha256) VALUES(?,?,?) ON DUPLICATE KEY UPDATE request_id=buyback_history_stores.request_id`, request, store, hash).Error != nil {
		return ErrUnavailable
	}
	if tx.Table("buyback_history_stores").Where("request_id=? AND store_id=? AND snapshot_sha256=?", request, store, hash).Count(&n).Error != nil || n != 1 {
		return ErrInput
	}
	return nil
}

func historicalQuery(tx *gorm.DB, store, id string) *gorm.DB {
	return tx.Table("buyback_requests r").Joins("JOIN buyback_history_stores h ON h.request_id=r.id").Joins("JOIN commerce_stores s ON s.id=h.store_id AND s.deleted_at IS NULL").Where("r.id=? AND r.deleted_at IS NULL AND h.store_id=?", id, store).Where("NOT EXISTS(SELECT 1 FROM customer_buyback_submissions b WHERE b.request_id=r.id)")
}

func readHistoricalRequest(tx *gorm.DB, store, id string) (RequestDetail, error) {
	var row requestRow
	if !validID(store, 191) || !validID(id, 191) {
		return RequestDetail{}, ErrMissing
	}
	e := historicalQuery(tx, store, id).Select(`r.id,r.status,r.contact_name,r.email,r.phone,r.return_address,r.items,r.currency_code,CAST(r.reference_amount AS CHAR) AS reference_amount,r.raw_reference_amount,CAST(r.final_amount AS CHAR) AS final_amount,r.raw_final_amount,r.shipping_instructions,r.tracking_number,r.return_tracking,r.payment_reference,r.review_note,r.history,r.created_at,r.updated_at,r.closed_at,r.deleted_at`).Take(&row).Error
	if e == gorm.ErrRecordNotFound {
		return RequestDetail{}, ErrMissing
	}
	if e != nil {
		return RequestDetail{}, ErrUnavailable
	}
	return projectRequest(tx, row, id)
}
