import { defineRouteConfig } from "@medusajs/admin-sdk"
import SEOReference from "../../../components/seo-reference"

const SEOSectionPage = () => <SEOReference section="traffic-sources" />
export default SEOSectionPage
export const config = defineRouteConfig({ label: "seo.reference.traffic-sources", translationNs: "translation", rank: 2 })
