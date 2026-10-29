// Package authorization checks explicit current grants for new accounts.
package authorization

const modelSHA = "1b974d153bffcdd42f27cd8d771a68f03f7fcfd7992fcc6c63145baf9ff9a18e"

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
	PriceWrite           Action = "price.write"
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
	return []Action{Read, Write, DeviceEnroll, DeviceIntake, DeviceRead, DeviceSourceWrite, InventoryRead, StockWrite, MailRead, MailWrite, OrderAmountRead, OrderFulfillmentRead, OrderPaymentRead, OrderRead, PriceRead, PriceWrite, ProductRead, ProductWrite, WarehouseIntakeRead, WarehouseIntakeWrite, WarehouseRead, WarehouseReturnRead, WarehouseReturnWrite, WarehouseWrite}
}
func known(action Action) bool {
	for _, value := range Actions() {
		if action == value {
			return true
		}
	}
	return false
}
