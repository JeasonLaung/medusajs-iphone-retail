import { defineRouteConfig } from "@medusajs/admin-sdk"
import DeviceWorkbench from "../../../components/device-workbench"

const DeviceSourcesPage = () => {
  return <DeviceWorkbench key="sources" mode="sources" />
}

export default DeviceSourcesPage

export const config = defineRouteConfig({ label: "deviceScan.sources", translationNs: "translation", rank: 4 })
