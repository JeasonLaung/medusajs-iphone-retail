import { defineRouteConfig } from "@medusajs/admin-sdk"
import DeviceWorkbench from "../../../components/device-workbench"

const DeviceIntakePage = () => {
  return <DeviceWorkbench key="intake" mode="intake" />
}

export default DeviceIntakePage

export const config = defineRouteConfig({ label: "deviceScan.intake", translationNs: "translation", rank: 1 })
