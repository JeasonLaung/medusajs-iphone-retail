import { defineRouteConfig } from "@medusajs/admin-sdk"
import DeviceWorkbench from "../../../components/device-workbench"

const DeviceLookupPage = () => {
  return <DeviceWorkbench key="lookup" mode="lookup" />
}

export default DeviceLookupPage

export const config = defineRouteConfig({ label: "deviceScan.lookup", translationNs: "translation", rank: 0 })
