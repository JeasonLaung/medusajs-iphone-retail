import { defineRouteConfig } from "@medusajs/admin-sdk"
import { useSearchParams } from "react-router-dom"
import SEOReference, { type SEOSection } from "../../components/seo-reference"

const SEOPage = () => {
  const [params] = useSearchParams()
  const aliases: Record<string, SEOSection> = { "crawler-visits": "crawler-logs", "traffic-stats": "traffic-sources", "traffic-visits": "traffic-sources", "citation-monitors": "citation-monitor", "citation-runs": "citation-monitor", "page-metas": "page-metas", settings: "settings", providers: "providers" }
  return <SEOReference section={aliases[params.get("tab") || ""] || "overview"} />
}
export default SEOPage
export const config = defineRouteConfig({ label: "seo.reference.title", translationNs: "translation", rank: 6 })
