package productreviews

import (
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Purchase struct {
	ID        string    `json:"id"`
	Number    string    `json:"number"`
	CreatedAt time.Time `json:"created_at"`
	Eligible  bool      `json:"eligible"`
}
type OwnReview struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	VerifiedPurchase bool   `json:"verified_purchase"`
}
type Purchases struct {
	Orders []Purchase `json:"orders"`
	Review *OwnReview `json:"review"`
	Count  int64      `json:"count"`
	Offset int        `json:"offset"`
	Limit  int        `json:"limit"`
}
type OrderReview struct {
	ProductID        string `json:"product_id"`
	Status           string `json:"status"`
	VerifiedPurchase bool   `json:"verified_purchase"`
}
type OrderContext struct {
	Eligible bool          `json:"eligible"`
	Reviews  []OrderReview `json:"reviews"`
}
type orderRecord struct {
	ID, Status      string
	DisplayID       *int64
	CustomDisplayID *string
	Version         int
	CreatedAt       time.Time
}

// The customer layer already holds the current verified account lock. This
// explicit ownership table is the only authority; orders.customer_id, source
// actors and matching email addresses never grant review purchase privileges.
func ownedOrder(tx *gorm.DB, account, store, id string) (orderRecord, error) {
	var row orderRecord
	e := tx.Table("orders o").Select("o.id,o.status,o.display_id,o.custom_display_id,o.version,o.created_at").Joins("JOIN customer_order_ownership ow ON ow.order_id=o.id AND ow.customer_id=? AND ow.store_id=?", account, store).Where("o.id=? AND o.deleted_at IS NULL", id).Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "o"}}).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return row, ErrOrder
	}
	if e != nil {
		return row, e
	}
	if row.Version < 1 {
		return row, ErrUnavailable
	}
	return row, nil
}
func orderFacts(tx *gorm.DB, order orderRecord) (bool, []string, error) {
	var payments []struct{ Status *string }
	// A dangling active link remains ineligible rather than silently disappearing.
	if e := tx.Table("order_payment_collections l").Select("p.status").Joins("LEFT JOIN payment_collections p ON p.id=l.payment_collection_id AND p.deleted_at IS NULL").Where("l.order_id=? AND l.deleted_at IS NULL", order.ID).Order("l.payment_collection_id").Clauses(clause.Locking{Strength: "SHARE"}).Limit(1001).Scan(&payments).Error; e != nil {
		return false, nil, e
	}
	if len(payments) > 1000 {
		return false, nil, ErrUnavailable
	}
	statuses := make([]*string, len(payments))
	for i, p := range payments {
		statuses[i] = p.Status
	}
	var items []struct{ ProductID *string }
	if e := tx.Table("order_items i").Select("li.product_id").Joins("JOIN order_line_items li ON li.id=i.item_id AND li.deleted_at IS NULL").Where("i.order_id=? AND i.version=? AND i.deleted_at IS NULL", order.ID, order.Version).Order("i.id").Limit(1001).Scan(&items).Error; e != nil {
		return false, nil, e
	}
	if len(items) > 1000 {
		return false, nil, ErrUnavailable
	}
	products := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		if item.ProductID != nil && *item.ProductID != "" && !seen[*item.ProductID] {
			seen[*item.ProductID] = true
			products = append(products, *item.ProductID)
		}
	}
	return OrderEligible(order.Status, statuses), products, nil
}
func containsProduct(ids []string, id string) bool {
	for _, s := range ids {
		if s == id {
			return true
		}
	}
	return false
}

func PurchasesTx(tx *gorm.DB, account, store string, q ListQuery) (Purchases, error) {
	out := Purchases{Orders: []Purchase{}, Offset: q.Offset, Limit: 20}
	if !idOK(account) || !idOK(store) || !idOK(q.ProductID) || q.Offset < 0 || q.Offset > 100000 || q.Limit != 20 {
		return out, ErrInput
	}
	if _, e := runtime(tx); e != nil {
		return out, e
	}
	base := func() *gorm.DB {
		return tx.Table("orders o").Joins("JOIN customer_order_ownership ow ON ow.order_id=o.id AND ow.customer_id=? AND ow.store_id=?", account, store).Where("o.deleted_at IS NULL")
	}
	if e := base().Count(&out.Count).Error; e != nil {
		return out, e
	}
	var rows []orderRecord
	if e := base().Select("o.id,o.status,o.display_id,o.custom_display_id,o.version,o.created_at").Order("o.created_at DESC,o.id DESC").Offset(q.Offset).Limit(20).Scan(&rows).Error; e != nil {
		return out, e
	}
	for _, row := range rows {
		eligible, products, e := orderFacts(tx, row)
		if e != nil {
			return out, e
		}
		number := ""
		if row.CustomDisplayID != nil && *row.CustomDisplayID != "" {
			number = *row.CustomDisplayID
		} else if row.DisplayID != nil {
			number = strconv.FormatInt(*row.DisplayID, 10)
		}
		out.Orders = append(out.Orders, Purchase{row.ID, number, row.CreatedAt, eligible && containsProduct(products, q.ProductID)})
	}
	var review OwnReview
	e := tx.Table("product_reviews").Select("id,status,verified_purchase").Where("account_id=? AND product_id=? AND deleted_at IS NULL", account, q.ProductID).Take(&review).Error
	if e == nil {
		out.Review = &review
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	return out, nil
}
func OrderContextTx(tx *gorm.DB, account, store, id string) (OrderContext, error) {
	out := OrderContext{Reviews: []OrderReview{}}
	if !idOK(account) || !idOK(store) || !idOK(id) {
		return out, ErrInput
	}
	if _, e := runtime(tx); e != nil {
		return out, e
	}
	order, e := ownedOrder(tx, account, store, id)
	if e != nil {
		return out, e
	}
	eligible, products, e := orderFacts(tx, order)
	if e != nil {
		return out, e
	}
	out.Eligible = eligible
	if len(products) > 0 {
		if e := tx.Table("product_reviews").Select("product_id,status,verified_purchase").Where("account_id=? AND product_id IN ? AND deleted_at IS NULL", account, products).Order("product_id").Scan(&out.Reviews).Error; e != nil {
			return out, e
		}
	}
	return out, nil
}
