// Package authorization checks explicit current grants for new accounts.
package authorization

const modelSHA = "d7f0a6b11cefe0238216f4705112a166f433710d19a09ed5db40f69c876dd9b4"

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
)

// Actions returns a fresh, exact and case-sensitive vocabulary. No wildcards.
func Actions() []Action {
	return []Action{Read, Write, InventoryRead, StockWrite, MailRead, MailWrite, OrderAmountRead, OrderFulfillmentRead, OrderPaymentRead, OrderRead, PriceRead, ProductRead, ProductWrite}
}
func known(action Action) bool {
	for _, value := range Actions() {
		if action == value {
			return true
		}
	}
	return false
}
