// Package authorization checks explicit current grants for new accounts.
package authorization

const modelSHA = "6b5e8c83320d52e0589f63c1e338dc94fddd7d439580d811c10a33dcfd6731e3"

type Action string

const (
	Read                 Action = "authorization.read"
	Write                Action = "authorization.write"
	MailRead             Action = "mail.read"
	MailWrite            Action = "mail.write"
	ProductRead          Action = "product.read"
	ProductWrite         Action = "product.write"
	PriceRead            Action = "price.read"
	InventoryRead        Action = "inventory.read"
	StockWrite           Action = "inventory.stock.write"
	OrderRead            Action = "order.read"
	OrderAmountRead      Action = "order.amount.read"
	OrderPaymentRead     Action = "order.payment.read"
	OrderFulfillmentRead Action = "order.fulfillment.read"
	WarehouseRead        Action = "warehouse.read"
	WarehouseWrite       Action = "warehouse.write"
)

// Actions returns a fresh, exact and case-sensitive vocabulary. No wildcards.
func Actions() []Action {
	return []Action{Read, Write, InventoryRead, StockWrite, MailRead, MailWrite, OrderAmountRead, OrderFulfillmentRead, OrderPaymentRead, OrderRead, PriceRead, ProductRead, ProductWrite, WarehouseRead, WarehouseWrite}
}
func known(action Action) bool {
	for _, value := range Actions() {
		if action == value {
			return true
		}
	}
	return false
}
