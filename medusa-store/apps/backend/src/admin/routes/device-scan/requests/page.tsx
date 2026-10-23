import { defineRouteConfig } from "@medusajs/admin-sdk"
import BuybackApplications from "../../../components/buyback-applications"

export default BuybackApplications
export const config = defineRouteConfig({ label: "buyback.adminTitle", translationNs: "translation", rank: 3 })
