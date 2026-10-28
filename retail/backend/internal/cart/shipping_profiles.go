package cart

import (
	"context"
	"sort"
	"strconv"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ShippingGroup is a private, current catalog projection. Saved product_id and
// browser-supplied profile IDs cannot establish a shipping relationship.
type ShippingGroup struct {
	ProfileID string
	Lines     []inventory.CartStockLine
}

// ReadShippingGroups participates in an owned cart transaction and rechecks
// possession/version before current reads. Every requires_shipping line must
// resolve exactly one active profile through its CURRENT variant/product.
// Missing/manual shipping lines or ambiguous links fail, never silently drop.
// This changes no data, reserves nothing and is not a standalone HTTP command.
func ReadShippingGroups(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string) ([]ShippingGroup, error) {
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return nil, err
	}
	var lines []struct {
		ID               string
		VariantID        *string
		Quantity         int64
		RequiresShipping bool
	}
	if tx.Table("cart_line_items").Select("id,variant_id,quantity,requires_shipping").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("id").Clauses(clause.Locking{Strength: "SHARE"}).Limit(1001).Scan(&lines).Error != nil || len(lines) > 1000 {
		return nil, ErrSchema
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, line := range lines {
		if !idPattern.MatchString(line.ID) || line.Quantity < 1 || line.Quantity > 1000000000 {
			return nil, ErrSchema
		}
		if !line.RequiresShipping {
			continue
		}
		if line.VariantID == nil || !idPattern.MatchString(*line.VariantID) {
			return nil, ErrItemUnavailable
		}
		if !seen[*line.VariantID] {
			seen[*line.VariantID] = true
			ids = append(ids, *line.VariantID)
		}
	}
	out := []ShippingGroup{}
	if len(ids) == 0 {
		return out, nil
	}
	// Current locking reads also defeat an older RR snapshot established by
	// an earlier phase of this same transaction.
	// Session keeps the lock clause but clones statements for each query;
	// catalog projections issue several independent relation queries.
	current := tx.Clauses(clause.Locking{Strength: "SHARE"}).Session(&gorm.Session{})
	variants, err := catalog.ReadCartVariants(current, ids, cart.SalesChannelID)
	if err != nil {
		return nil, ErrItemUnavailable
	}
	products := []string{}
	productSeen := map[string]bool{}
	for _, id := range ids {
		v := variants[id]
		if !productSeen[v.ProductID] {
			productSeen[v.ProductID] = true
			products = append(products, v.ProductID)
		}
	}
	sort.Strings(products)
	var links []struct {
		ProductID, ShippingProfileID string
		ProfileAvailable             bool
	}
	if current.Table("catalog_product_shipping_profiles l").Select("l.product_id,l.shipping_profile_id,(p.id IS NOT NULL) AS profile_available").Joins("LEFT JOIN fulfillment_shipping_profiles p ON p.id=l.shipping_profile_id AND p.deleted_at IS NULL").Where("l.product_id IN ? AND l.deleted_at IS NULL", products).Order("l.product_id,l.shipping_profile_id").Limit(2001).Scan(&links).Error != nil || len(links) > 2000 {
		return nil, ErrSchema
	}
	byProduct := map[string]string{}
	for _, link := range links {
		if !link.ProfileAvailable || !idPattern.MatchString(link.ShippingProfileID) || byProduct[link.ProductID] != "" {
			return nil, ErrItemUnavailable
		}
		byProduct[link.ProductID] = link.ShippingProfileID
	}
	groups := map[string][]inventory.CartStockLine{}
	for _, line := range lines {
		if !line.RequiresShipping {
			continue
		}
		profile := byProduct[variants[*line.VariantID].ProductID]
		if profile == "" {
			return nil, ErrItemUnavailable
		}
		groups[profile] = append(groups[profile], inventory.CartStockLine{ID: line.ID, VariantID: *line.VariantID, Quantity: strconv.FormatInt(line.Quantity, 10)})
	}
	profiles := []string{}
	for profile := range groups {
		profiles = append(profiles, profile)
	}
	sort.Strings(profiles)
	for _, profile := range profiles {
		out = append(out, ShippingGroup{ProfileID: profile, Lines: groups[profile]})
	}
	return out, nil
}
