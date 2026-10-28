package cart

import (
	"context"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RefreshItemSnapshots implements the non-price preparation phase of the
// original cart refresh. Existing descriptions, explicit empty strings,
// options, shipping and discount flags remain saved snapshots. Product ID and
// gift-card status follow the current variant; null metadata becomes {}.
// Caller owns the transaction, MUST roll back after any error and must finish
// pricing/tax/promotion/shipping, version and durable receipt before commit.
// This helper neither commits nor exposes a standalone HTTP capability.
func RefreshItemSnapshots(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string) (int, error) {
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return 0, err
	}
	var lines []struct {
		ID        string
		VariantID *string
		Quantity  int64
	}
	if tx.Table("cart_line_items").Select("id,variant_id,quantity").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1001).Scan(&lines).Error != nil || len(lines) > 1000 {
		return 0, ErrSchema
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, line := range lines {
		if !idPattern.MatchString(line.ID) || line.Quantity < 1 || line.Quantity > 1000000000 {
			return 0, ErrSchema
		}
		if line.VariantID != nil && !seen[*line.VariantID] {
			ids = append(ids, *line.VariantID)
			seen[*line.VariantID] = true
		}
	}
	variants, err := catalog.ReadCartVariants(tx, ids, cart.SalesChannelID)
	if err != nil {
		return 0, ErrItemUnavailable
	}
	for _, line := range lines {
		var r *gorm.DB
		if line.VariantID == nil {
			// Without a variant the original prepare defaults is_giftcard to
			// false. Legacy product references never resolve a login or product.
			r = tx.Exec(`UPDATE cart_line_items SET is_giftcard=FALSE,metadata=CASE WHEN metadata IS NULL OR JSON_TYPE(metadata)='NULL' THEN '{}' ELSE metadata END,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND cart_id=? AND deleted_at IS NULL`, line.ID, cart.ID)
		} else {
			v, ok := variants[*line.VariantID]
			if !ok {
				return 0, ErrItemUnavailable
			}
			r = tx.Exec(`UPDATE cart_line_items SET
 product_id=?,subtitle=COALESCE(subtitle,?),thumbnail=COALESCE(thumbnail,?),
 product_title=COALESCE(product_title,?),product_description=COALESCE(product_description,?),
 product_subtitle=COALESCE(product_subtitle,?),product_type=COALESCE(product_type,?),
 product_type_id=COALESCE(product_type_id,?),product_collection=COALESCE(product_collection,?),
 product_handle=COALESCE(product_handle,?),variant_sku=COALESCE(variant_sku,?),
 variant_barcode=COALESCE(variant_barcode,?),variant_title=COALESCE(variant_title,?),
 is_giftcard=?,metadata=CASE WHEN metadata IS NULL OR JSON_TYPE(metadata)='NULL' THEN '{}' ELSE metadata END,
 updated_at=UTC_TIMESTAMP(6) WHERE id=? AND cart_id=? AND deleted_at IS NULL`,
				v.ProductID, v.Title, v.Thumbnail, v.ProductTitle, v.ProductDescription, v.ProductSubtitle,
				v.ProductType, v.ProductTypeID, v.ProductCollection, v.ProductHandle, v.SKU, v.Barcode,
				v.Title, v.IsGiftcard, line.ID, cart.ID)
		}
		if r.Error != nil || r.RowsAffected != 1 {
			return 0, ErrStorage
		}
	}
	return len(lines), nil
}
