package catalog

import (
	"encoding/json"

	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
)

// PromotionVariant is the current variant relation selected by cart refresh.
// Product and numeric nulls are preserved, without product-level fallback.
// These internal facts do not grant purchase or shipping availability.
type PromotionVariant struct {
	ID       string                   `json:"id"`
	Product  *PromotionVariantProduct `json:"product"`
	Weight   *json.Number             `json:"weight"`
	Length   *json.Number             `json:"length"`
	Height   *json.Number             `json:"height"`
	Width    *json.Number             `json:"width"`
	Material *string                  `json:"material"`
}

type PromotionVariantProduct struct {
	ID              string             `json:"id"`
	ShippingProfile *PromotionRelation `json:"shipping_profile"`
}

// ReadPromotionVariants stays in the caller's transaction snapshot. Missing or
// deleted variants are absent; missing/deleted products are null relations.
func ReadPromotionVariants(tx *gorm.DB, ids []string) (map[string]PromotionVariant, error) {
	if tx == nil || tx.Statement == nil || len(ids) > 1000 {
		return nil, ErrCartVariant
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, ErrCartVariant
	}
	if Ready(tx.Statement.Context, tx) != nil {
		return nil, ErrSchema
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if !validProductID(id) {
			return nil, ErrCartVariant
		}
		if !seen[id] {
			unique = append(unique, id)
			seen[id] = true
		}
	}
	out := map[string]PromotionVariant{}
	if len(unique) == 0 {
		return out, nil
	}
	var rows []struct {
		ID                                                 string
		ProductID, Weight, Length, Height, Width, Material *string
	}
	if tx.Table("catalog_variants v").Select("v.id,p.id AS product_id,CAST(v.weight AS CHAR) AS weight,CAST(v.length AS CHAR) AS length,CAST(v.height AS CHAR) AS height,CAST(v.width AS CHAR) AS width,v.material").Joins("LEFT JOIN catalog_products p ON p.id=v.product_id AND p.deleted_at IS NULL").Where("v.id IN ? AND v.deleted_at IS NULL", unique).Order("v.id").Limit(1001).Scan(&rows).Error != nil || len(rows) > 1000 {
		return nil, ErrCartVariant
	}
	products := []string{}
	seenProducts := map[string]bool{}
	for _, row := range rows {
		if !validProductID(row.ID) || row.ProductID != nil && !validProductID(*row.ProductID) {
			return nil, ErrCartVariant
		}
		v := PromotionVariant{ID: row.ID, Material: row.Material}
		for _, number := range []struct {
			source *string
			target **json.Number
		}{
			{row.Weight, &v.Weight}, {row.Length, &v.Length}, {row.Height, &v.Height}, {row.Width, &v.Width},
		} {
			if number.source == nil {
				continue
			}
			value, err := totals.Resolve(totals.Number{Decimal: *number.source})
			if err != nil {
				return nil, ErrCartVariant
			}
			n := json.Number(value)
			*number.target = &n
		}
		if row.ProductID != nil {
			v.Product = &PromotionVariantProduct{ID: *row.ProductID}
			if !seenProducts[*row.ProductID] {
				products = append(products, *row.ProductID)
				seenProducts[*row.ProductID] = true
			}
		}
		out[v.ID] = v
	}
	if len(products) == 0 {
		return out, nil
	}
	var links []struct{ ProductID, ID string }
	if tx.Table("catalog_product_shipping_profiles l").Select("l.product_id,p.id").Joins("JOIN fulfillment_shipping_profiles p ON p.id=l.shipping_profile_id AND p.deleted_at IS NULL").Where("l.product_id IN ? AND l.deleted_at IS NULL", products).Order("l.product_id,p.id").Limit(1001).Scan(&links).Error != nil || len(links) > 1000 {
		return nil, ErrCartVariant
	}
	profiles := map[string]string{}
	for _, link := range links {
		if !seenProducts[link.ProductID] || !validProductID(link.ID) || profiles[link.ProductID] != "" {
			// A singular relation must never silently choose among live links.
			return nil, ErrCartVariant
		}
		profiles[link.ProductID] = link.ID
	}
	for id, v := range out {
		if v.Product != nil {
			if profile := profiles[v.Product.ID]; profile != "" {
				v.Product.ShippingProfile = &PromotionRelation{ID: profile}
			}
		}
		out[id] = v
	}
	return out, nil
}
