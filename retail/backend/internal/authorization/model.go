// Package authorization checks explicit current grants for new accounts.
package authorization

const modelSHA = "303322599081b800a978b7a7b2b7c25f4e4f102fff518e73639d07b42609140a"

type Action string

const (
	ProductRead          Action = "product.read"
	ProductWrite         Action = "product.write"
	PriceRead            Action = "price.read"
	InventoryRead        Action = "inventory.read"
	StockWrite           Action = "inventory.stock.write"
	OrderRead            Action = "order.read"
	OrderAmountRead      Action = "order.amount.read"
	OrderPaymentRead     Action = "order.payment.read"
	OrderFulfillmentRead Action = "order.fulfillment.read"
)

// Actions returns a fresh, exact and case-sensitive vocabulary. No wildcards.
func Actions() []Action {
	return []Action{InventoryRead, StockWrite, OrderAmountRead, OrderFulfillmentRead, OrderPaymentRead, OrderRead, PriceRead, ProductRead, ProductWrite}
}
func known(action Action) bool {
	for _, value := range Actions() {
		if action == value {
			return true
		}
	}
	return false
}
