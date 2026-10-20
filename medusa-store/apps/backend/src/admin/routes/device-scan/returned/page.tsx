import { defineRouteConfig } from "@medusajs/admin-sdk"
import DeviceWorkbench from "../../../components/device-workbench"
const Page = () => <DeviceWorkbench mode="lookup" category="returned" />
export default Page
export const config = defineRouteConfig({ label: "deviceScan.returnedDevices", translationNs: "translation", rank: 1 })
