// Package authorization checks explicit current grants for new accounts.
package authorization

const modelSHA = "b640b0796228b6e305d09e2979735b58ab7b3b28c474e4a36d1b32e1a197e47e"

type Action string

const SEORead Action = "seo.read"
const SEOWrite Action = "seo.write"

const StorePoliciesRead Action = "store.policies.read"
const StorePoliciesWrite Action = "store.policies.write"

const (
	BuybackRead              Action = "buyback.read"
	BuybackWrite             Action = "buyback.write"
	OrderAfterSalesWrite     Action = "order.after_sales.write"
	OrderAllocationWrite     Action = "order.allocation.write"
	WarehouseAllocationWrite Action = "warehouse.allocation.write"
	WarehouseShipmentWrite   Action = "warehouse.shipment.write"
	WarehouseIntakeRead      Action = "warehouse.intake.read"
	WarehouseIntakeWrite     Action = "warehouse.intake.write"
	WarehouseReturnRead      Action = "warehouse.return.read"
	WarehouseReturnWrite     Action = "warehouse.return.write"
	DeviceIntake             Action = "device.intake"
	DeviceSourceWrite        Action = "device.source.write"
	DeviceRead               Action = "device.read"
	DeviceEnroll             Action = "device.enroll"
	Read                     Action = "authorization.read"
	Write                    Action = "authorization.write"
	MailRead                 Action = "mail.read"
	MailWrite                Action = "mail.write"
	ProductRead              Action = "product.read"
	ProductWrite             Action = "product.write"
	PriceRead                Action = "price.read"
	PriceWrite               Action = "price.write"
	InventoryRead            Action = "inventory.read"
	StockWrite               Action = "inventory.stock.write"
	OrderRead                Action = "order.read"
	OrderAmountRead          Action = "order.amount.read"
	OrderRefundWrite         Action = "order.refund.write"
	OrderPaymentRead         Action = "order.payment.read"
	OrderFulfillmentRead     Action = "order.fulfillment.read"
	WarehouseRead            Action = "warehouse.read"
	WarehouseWrite           Action = "warehouse.write"
)

// Actions returns a fresh, exact and case-sensitive vocabulary. No wildcards.
func Actions() []Action {
	return []Action{Read, Write, BuybackRead, BuybackWrite, DeviceEnroll, DeviceIntake, DeviceRead, DeviceSourceWrite, InventoryRead, StockWrite, MailRead, MailWrite, OrderAfterSalesWrite, OrderAllocationWrite, OrderAmountRead, OrderFulfillmentRead, OrderPaymentRead, OrderRead, OrderRefundWrite, PriceRead, PriceWrite, ProductRead, ProductWrite, SEORead, SEOWrite, StorePoliciesRead, StorePoliciesWrite, WarehouseAllocationWrite, WarehouseIntakeRead, WarehouseIntakeWrite, WarehouseRead, WarehouseReturnRead, WarehouseReturnWrite, WarehouseShipmentWrite, WarehouseWrite}
}
func known(action Action) bool {
	for _, value := range Actions() {
		if action == value {
			return true
		}
	}
	return false
}
