// Package authorization checks explicit current grants for new accounts.
package authorization

const modelSHA = "a76ecdc0d6a7b657aa55197651fff00a98016e6d2bf36ce3a85cc5ee6f9e5e65"

type Action string

const (
	OrderAllocationWrite Action = "order.allocation.write"
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
	return []Action{Read, Write, DeviceEnroll, DeviceIntake, DeviceRead, DeviceSourceWrite, InventoryRead, StockWrite, MailRead, MailWrite, OrderAllocationWrite, OrderAmountRead, OrderFulfillmentRead, OrderPaymentRead, OrderRead, PriceRead, PriceWrite, ProductRead, ProductWrite, WarehouseIntakeRead, WarehouseIntakeWrite, WarehouseRead, WarehouseReturnRead, WarehouseReturnWrite, WarehouseWrite}
}
func known(action Action) bool {
	for _, value := range Actions() {
		if action == value {
			return true
		}
	}
	return false
}
