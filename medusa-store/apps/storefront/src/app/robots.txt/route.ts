import { getSEO, seoOrigin } from "@lib/data/seo"
export const dynamic = "force-dynamic"
export async function GET() {
  const { settings } = await getSEO()
  return new Response(settings["seo.robots_txt"] || `User-agent: *\nAllow: /\nDisallow: /*/account\nDisallow: /*/checkout\nDisallow: /*/cart\nDisallow: /*/order\nDisallow: /api/\nSitemap: ${seoOrigin(settings)}/sitemap.xml\n`, { headers: { "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "no-store" } })
}
