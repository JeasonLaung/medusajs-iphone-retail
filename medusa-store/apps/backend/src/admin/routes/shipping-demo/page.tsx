import { defineRouteConfig } from "@medusajs/admin-sdk"
import DhlOfficialTracking from "../../components/dhl-official-tracking"

const ShippingDemo = () => <DhlOfficialTracking />
export const config = defineRouteConfig({})
export default ShippingDemo
