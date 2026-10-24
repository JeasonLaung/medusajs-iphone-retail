import { defineRouteConfig } from "@medusajs/admin-sdk"
import ShipmentTracking from "../../components/shipment-tracking"
import demo from "../../../../../../shared/shipping-demo.json"

const ShippingDemo = () => {
  return <ShipmentTracking shipments={[demo]} demonstration />
}

export const config = defineRouteConfig({ label: "Shipping demo / 物流演示" })

export default ShippingDemo
