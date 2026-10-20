import { defineRouteConfig } from "@medusajs/admin-sdk"
import DeviceWorkbench from "../../../components/device-workbench"

const DeviceOrdersPage = () => {
  return <DeviceWorkbench key="orders" mode="orders" />
}

export default DeviceOrdersPage

export const config = defineRouteConfig({ label: "deviceScan.orders", translationNs: "translation", rank: 4 })
