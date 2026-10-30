package buyback

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/pricing"
	"gorm.io/gorm"
)

var ErrMissing = errors.New("buyback request not found")

type PublicPhoto struct {
	ID           string    `json:"id"`
	RequestID    string    `json:"request_id"`
	ItemID       string    `json:"item_id"`
	PhotoType    string    `json:"photo_type"`
	UploadedAt   time.Time `json:"uploaded_at"`
	ReviewStatus string    `json:"review_status"`
	ReviewNote   string    `json:"review_note"`
	MimeType     string    `json:"mime_type"`
}

type RequestDetail struct {
	CreatedRequest
	// Original workflow history may also include note and photo_id.
	History   []json.RawMessage `json:"history"`
	Photos    []PublicPhoto     `json:"photos"`
	ClosedAt  *time.Time        `json:"closed_at"`
	DeletedAt *time.Time        `json:"deleted_at"`
}

type requestRow struct {
	ID, Status, ContactName, Email, Phone, ReturnAddress, Items, CurrencyCode                   string
	ReferenceAmount, RawReferenceAmount, FinalAmount, RawFinalAmount                            *string
	ShippingInstructions, TrackingNumber, ReturnTracking, PaymentReference, ReviewNote, History string
	CreatedAt, UpdatedAt                                                                        time.Time
	ClosedAt, DeletedAt                                                                         *time.Time
}

func requestAmount(decimal, raw *string) (*json.Number, error) {
	if decimal == nil {
		if raw != nil {
			return nil, ErrUnavailable
		}
		return nil, nil
	}
	if raw == nil {
		return nil, ErrUnavailable
	}
	amount, e := pricing.ConfiguredAmount(*decimal, *raw)
	if e != nil || strings.HasPrefix(amount, "-") {
		return nil, ErrUnavailable
	}
	n := json.Number(amount)
	return &n, nil
}

// ReadRequest runs inside the caller's authenticated transaction. Explicit new
// account/store ownership is mandatory; historical matching IDs/emails grant none.
// Never replay result_json: the page must observe current workflow state.
func ReadRequest(ctx context.Context, tx *gorm.DB, storeID, accountID, id string) (RequestDetail, error) {
	var out RequestDetail
	if tx == nil || !validID(storeID, 191) || !validID(accountID, 80) {
		return out, ErrUnavailable
	}
	if !validID(id, 191) {
		return out, ErrMissing
	}
	tx = tx.WithContext(ctx)
	var row requestRow
	e := tx.Table("buyback_requests r").Select(`r.id,r.status,r.contact_name,r.email,r.phone,r.return_address,r.items,r.currency_code,CAST(r.reference_amount AS CHAR) AS reference_amount,r.raw_reference_amount,CAST(r.final_amount AS CHAR) AS final_amount,r.raw_final_amount,r.shipping_instructions,r.tracking_number,r.return_tracking,r.payment_reference,r.review_note,r.history,r.created_at,r.updated_at,r.closed_at,r.deleted_at`).Joins("JOIN customer_buyback_submissions b ON b.request_id=r.id AND b.customer_id=r.customer_id").Joins("JOIN commerce_stores s ON s.id=b.store_id AND s.deleted_at IS NULL").Where("r.id=? AND r.deleted_at IS NULL AND b.customer_id=? AND b.store_id=?", id, accountID, storeID).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return out, ErrMissing
	}
	if e != nil {
		return out, ErrUnavailable
	}
	return projectRequest(tx, row, id)
}

// Projection is shared only after the caller has proved customer or platform
// historical scope. It is not an authorization entry point.
func projectRequest(tx *gorm.DB, row requestRow, id string) (RequestDetail, error) {
	var out RequestDetail
	var e error
	if len(row.Items) > 262144 || len(row.ReturnAddress) > 16384 || len(row.History) > 1048576 {
		return out, ErrUnavailable
	}
	out.CreatedRequest = CreatedRequest{ID: row.ID, Status: row.Status, ContactName: row.ContactName, Email: row.Email, Phone: row.Phone, CurrencyCode: row.CurrencyCode, ShippingInstructions: row.ShippingInstructions, TrackingNumber: row.TrackingNumber, ReturnTracking: row.ReturnTracking, PaymentReference: row.PaymentReference, ReviewNote: row.ReviewNote, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, RetentionDays: 180}
	out.ClosedAt = row.ClosedAt
	out.DeletedAt = row.DeletedAt
	if json.Unmarshal([]byte(row.Items), &out.Items) != nil || len(out.Items) < 1 || len(out.Items) > 10 || json.Unmarshal([]byte(row.ReturnAddress), &out.ReturnAddress) != nil || json.Unmarshal([]byte(row.History), &out.History) != nil || out.History == nil {
		return RequestDetail{}, ErrUnavailable
	}
	out.ReferenceAmount, e = requestAmount(row.ReferenceAmount, row.RawReferenceAmount)
	if e != nil {
		return RequestDetail{}, e
	}
	out.FinalAmount, e = requestAmount(row.FinalAmount, row.RawFinalAmount)
	if e != nil {
		return RequestDetail{}, e
	}
	// Same oldest-first Map replacement as original latestPhotos; replacing a
	// version retains its first slot, including ties resolved by generated ID.
	var photos []PublicPhoto
	if tx.Table("buyback_photos").Select("id,request_id,item_id,photo_type,uploaded_at,review_status,review_note,mime_type").Where("request_id=? AND deleted_at IS NULL", id).Order("uploaded_at,id").Limit(10001).Scan(&photos).Error != nil || len(photos) > 10000 {
		return RequestDetail{}, ErrUnavailable
	}
	out.Photos = []PublicPhoto{}
	positions := map[string]int{}
	for _, photo := range photos {
		key := photo.ItemID + ":" + photo.PhotoType
		if index, ok := positions[key]; ok {
			out.Photos[index] = photo
		} else {
			positions[key] = len(out.Photos)
			out.Photos = append(out.Photos, photo)
		}
	}
	return out, nil
}
