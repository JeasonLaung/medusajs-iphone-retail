// Package authorization checks explicit current grants for new accounts.
package authorization

const modelSHA = "10e78cdc98fa65a573111d95b726fa062cca66a1d787006cb1b2c9ca92bad7c5"

type Action string

const (
	WarehouseIntakeRead  Action = "warehouse.intake.read"
	WarehouseIntakeWrite Action = "warehouse.intake.write"
	WarehouseReturnRead  Action = "warehouse.return.read"
	WarehouseReturnWrite Action = "warehouse.return.write"
	DeviceIntake         Action = "device.intake"
	DeviceSourceWrite    Action = "device.source.write"
	DeviceRead           Action = "device.read"
	DeviceEnroll         Action = "device.enroll"
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
	return []Action{Read, Write, DeviceEnroll, DeviceIntake, DeviceRead, DeviceSourceWrite, InventoryRead, StockWrite, MailRead, MailWrite, OrderAmountRead, OrderFulfillmentRead, OrderPaymentRead, OrderRead, PriceRead, ProductRead, ProductWrite, WarehouseIntakeRead, WarehouseIntakeWrite, WarehouseRead, WarehouseReturnRead, WarehouseReturnWrite, WarehouseWrite}
}
func known(action Action) bool {
	for _, value := range Actions() {
		if action == value {
			return true
		}
	}
	return false
}
