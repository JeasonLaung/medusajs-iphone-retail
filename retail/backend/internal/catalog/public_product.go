package catalog

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RequirePublicProduct shares the catalog's trusted shop/channel scope with
// product reviews. A shared product lock keeps publication stable during a
// submission; no caller-controlled sales channel is accepted.
func RequirePublicProduct(ctx context.Context, tx *gorm.DB, storeID, productID string) error {
	h := StoreHandler{DB: tx, StoreID: storeID}
	channel, e := h.channel(tx.WithContext(ctx))
	if e != nil {
		return e
	}
	var row struct{ ID string }
	return publicProducts(tx, channel).Select("p.id").Where("p.id=?", productID).Clauses(clause.Locking{Strength: "SHARE", Table: clause.Table{Name: "p"}}).Take(&row).Error
}
