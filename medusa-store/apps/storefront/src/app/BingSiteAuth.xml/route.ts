import { getSEO } from "@lib/data/seo"
export const dynamic = "force-dynamic"
export async function GET() {
  const { settings: values } = await getSEO()
  if (values["bing.verification_enabled"] !== "true" || !values["bing.site_auth_xml"]) return new Response(null, { status: 404 })
  return new Response(values["bing.site_auth_xml"], { headers: { "Content-Type": "application/xml; charset=utf-8", "Cache-Control": "no-store" } })
}
