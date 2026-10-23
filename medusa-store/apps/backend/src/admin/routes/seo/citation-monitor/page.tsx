import { defineRouteConfig } from "@medusajs/admin-sdk"
import SEOReference from "../../../components/seo-reference"

const SEOSectionPage = () => <SEOReference section="citation-monitor" />
export default SEOSectionPage
export const config = defineRouteConfig({ label: "seo.reference.citation-monitor", translationNs: "translation", rank: 4 })
