import { defineRouteConfig } from "@medusajs/admin-sdk"
import DeviceWorkbench from "../../../components/device-workbench"

const DeviceInspectionPage = () => {
  return <DeviceWorkbench key="inspection" mode="inspection" />
}

export default DeviceInspectionPage

export const config = defineRouteConfig({ label: "deviceScan.afterSales", translationNs: "translation", rank: 3 })
