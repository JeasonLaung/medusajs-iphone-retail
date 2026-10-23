import { defineRouteConfig } from "@medusajs/admin-sdk"
import BuybackPriceCatalog from "../../../components/buyback-price-catalog"
export default BuybackPriceCatalog
export const config = defineRouteConfig({ label: "buyback.catalog.title", translationNs: "translation", rank: 4 })
