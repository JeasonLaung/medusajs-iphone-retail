import { defineRouteConfig } from "@medusajs/admin-sdk"
import DeviceWorkbench from "../../../components/device-workbench"
const Page = () => <DeviceWorkbench mode="lookup" category="recycled" />
export default Page
export const config = defineRouteConfig({ label: "deviceScan.recycledDevices", translationNs: "translation", rank: 2 })
