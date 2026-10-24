import { defineRouteConfig } from "@medusajs/admin-sdk"
import DhlOfficialTracking from "../../components/dhl-official-tracking"

const ShippingDemo = () => <DhlOfficialTracking />
export const config = defineRouteConfig({ label: "Shipping demo / 物流演示" })
export default ShippingDemo
