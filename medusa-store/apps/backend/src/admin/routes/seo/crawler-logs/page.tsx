import { defineRouteConfig } from "@medusajs/admin-sdk"
import SEOReference from "../../../components/seo-reference"

const SEOSectionPage = () => <SEOReference section="crawler-logs" />
export default SEOSectionPage
export const config = defineRouteConfig({ label: "seo.reference.crawler-logs", translationNs: "translation", rank: 1 })
