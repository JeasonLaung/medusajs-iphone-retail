import { defineRouteConfig } from "@medusajs/admin-sdk"
import SEOReference from "../../../components/seo-reference"

const SEOSectionPage = () => <SEOReference section="providers" />
export default SEOSectionPage
export const config = defineRouteConfig({ label: "seo.reference.providers", translationNs: "translation", rank: 3 })
