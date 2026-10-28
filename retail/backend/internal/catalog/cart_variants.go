package catalog

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

var ErrCartVariant = errors.New("cart variant unavailable")

// CartVariant is a server-only projection. No source metadata, provider data,
// client-selected flags or browser price is accepted as a purchase snapshot.
type CartVariant struct {
	ID, ProductID, Title, ProductTitle, ProductHandle                                                           string
	SKU, Barcode, Thumbnail, ProductSubtitle, ProductDescription, ProductTypeID, ProductType, ProductCollection *string
	ManageInventory, AllowBackorder, Complete, Discountable, IsGiftcard, RequiresShipping                       bool
	OptionValues                                                                                                string
}

// ReadCartVariants shares public product/channel and configuration membership
// checks with the existing directory. Caller owns a single SQL snapshot.
func ReadCartVariants(tx *gorm.DB, ids []string, channel string) (map[string]CartVariant, error) {
	if tx == nil || len(ids) > 1000 || !validProductID(channel) {
		return nil, ErrCartVariant
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		if !validProductID(id) || wanted[id] {
			return nil, ErrCartVariant
		}
		wanted[id] = true
	}
	out := map[string]CartVariant{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []CartVariant
	projection := `v.id,v.product_id,v.title,v.sku,v.barcode,COALESCE(v.thumbnail,p.thumbnail) AS thumbnail,v.manage_inventory,v.allow_backorder,p.title AS product_title,p.handle AS product_handle,p.subtitle AS product_subtitle,p.description AS product_description,p.type_id AS product_type_id,t.value AS product_type,col.title AS product_collection,p.discountable,p.is_giftcard,
 (EXISTS(SELECT 1 FROM catalog_product_shipping_profiles ps JOIN fulfillment_shipping_profiles sp ON sp.id=ps.shipping_profile_id AND sp.deleted_at IS NULL WHERE ps.product_id=p.id AND ps.deleted_at IS NULL)
 OR EXISTS(SELECT 1 FROM catalog_variant_inventory_items vi JOIN inventory_items i ON i.id=vi.inventory_item_id AND i.deleted_at IS NULL WHERE vi.variant_id=v.id AND vi.deleted_at IS NULL AND i.requires_shipping=TRUE)) AS requires_shipping`
	if publicProducts(tx, channel).Select(projection).Joins("JOIN catalog_variants v ON v.product_id=p.id AND v.deleted_at IS NULL").Joins("LEFT JOIN catalog_product_types t ON t.id=p.type_id AND t.deleted_at IS NULL").Joins("LEFT JOIN catalog_collections col ON col.id=p.collection_id AND col.deleted_at IS NULL").Where("v.id IN ?", ids).Order("v.id").Limit(1001).Scan(&rows).Error != nil || len(rows) != len(ids) {
		return nil, ErrCartVariant
	}
	budget := 20000
	products := map[string]StoreProduct{}
	for _, r := range rows {
		p, ok := products[r.ProductID]
		if !ok {
			p = StoreProduct{ID: r.ProductID}
			if publicProductRelations(tx, &p, &budget) != nil {
				return nil, ErrCartVariant
			}
			products[r.ProductID] = p
		}
		values := map[string]string{}
		for _, v := range p.Variants {
			if v.ID != r.ID {
				continue
			}
			r.Complete = v.ConfigurationComplete
			for _, choice := range v.Options {
				for _, option := range p.Options {
					if option.ID == choice.OptionID {
						if _, duplicate := values[option.Title]; duplicate {
							return nil, ErrCartVariant
						}
						values[option.Title] = choice.Value
					}
				}
			}
		}
		b, err := json.Marshal(values)
		if err != nil || !r.Complete {
			return nil, ErrCartVariant
		}
		r.OptionValues = string(b)
		out[r.ID] = r
	}
	return out, nil
}
