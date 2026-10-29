package catalog

import (
	"encoding/json"

	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
)

type PromotionRelation struct {
	ID string `json:"id"`
}

// PromotionProduct is the current product relation selected by the original
// cart refresh graph, separate from saved cart-line presentation fields.
// This internal projection grants no purchase availability or discount right.
type PromotionProduct struct {
	ID           string              `json:"id"`
	IsGiftcard   bool                `json:"is_giftcard"`
	CollectionID *string             `json:"collection_id"`
	TypeID       *string             `json:"type_id"`
	Weight       *json.Number        `json:"weight"`
	Categories   []PromotionRelation `json:"categories"`
	Tags         []PromotionRelation `json:"tags"`
}

// ReadPromotionProducts stays in the caller's snapshot. Missing/deleted products
// are absent so the assembler can represent a null relation, without deleting
// saved lines. Draft/private category relations are retained: the original
// internal graph is not the storefront's public taxonomy filter.
func ReadPromotionProducts(tx *gorm.DB, ids []string) (map[string]PromotionProduct, error) {
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
	out := map[string]PromotionProduct{}
	if len(unique) == 0 {
		return out, nil
	}
	var rows []struct {
		ID                           string
		IsGiftcard                   bool
		CollectionID, TypeID, Weight *string
	}
	if tx.Table("catalog_products").Select("id,is_giftcard,collection_id,type_id,CAST(weight AS CHAR) AS weight").Where("id IN ? AND deleted_at IS NULL", unique).Order("id").Limit(1001).Scan(&rows).Error != nil || len(rows) > 1000 {
		return nil, ErrCartVariant
	}
	active := []string{}
	for _, row := range rows {
		if !validProductID(row.ID) || row.CollectionID != nil && !validProductID(*row.CollectionID) || row.TypeID != nil && !validProductID(*row.TypeID) {
			return nil, ErrCartVariant
		}
		p := PromotionProduct{ID: row.ID, IsGiftcard: row.IsGiftcard, CollectionID: row.CollectionID, TypeID: row.TypeID, Categories: []PromotionRelation{}, Tags: []PromotionRelation{}}
		if row.Weight != nil {
			value, err := totals.Resolve(totals.Number{Decimal: *row.Weight})
			if err != nil {
				return nil, ErrCartVariant
			}
			n := json.Number(value)
			p.Weight = &n
		}
		out[row.ID] = p
		active = append(active, row.ID)
	}
	if len(active) == 0 {
		return out, nil
	}
	for _, spec := range []struct {
		table, target, key string
		category           bool
	}{
		{"catalog_category_products", "catalog_categories", "category_id", true},
		{"catalog_product_tags", "catalog_tags", "tag_id", false},
	} {
		var links []struct{ ProductID, ID string }
		if tx.Table(spec.table+" l").Select("l.product_id,t.id").Joins("JOIN "+spec.target+" t ON t.id=l."+spec.key+" AND t.deleted_at IS NULL").Where("l.product_id IN ?", active).Order("l.product_id,t.id").Limit(10001).Scan(&links).Error != nil || len(links) > 10000 {
			return nil, ErrCartVariant
		}
		for _, link := range links {
			p, ok := out[link.ProductID]
			if !ok || !validProductID(link.ID) {
				return nil, ErrCartVariant
			}
			if spec.category {
				p.Categories = append(p.Categories, PromotionRelation{ID: link.ID})
				if len(p.Categories) > 1000 {
					return nil, ErrCartVariant
				}
			} else {
				p.Tags = append(p.Tags, PromotionRelation{ID: link.ID})
				if len(p.Tags) > 1000 {
					return nil, ErrCartVariant
				}
			}
			out[link.ProductID] = p
		}
	}
	return out, nil
}
