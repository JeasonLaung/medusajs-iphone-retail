import { getSEO, seoOrigin } from "@lib/data/seo"
export const dynamic = "force-dynamic"
export async function GET() {
  const { settings: values } = await getSEO(), origin = seoOrigin(values), home = values["seo.home_path"] || "/dk-en"
  const content = values["seo.llms_txt"] || `# ${values["seo.site_name"] || "EuroRefurb"}\n\n${values["seo.default_description"] || "Refurbished iPhones. Product specifications, prices and availability are provided on each product page."}\n\n## Pages\n- [Store](${origin}${home}/store)\n- [About](${origin}${home}/about)\n- [Help](${origin}${home}/help)\n- [Sitemap](${origin}/sitemap.xml)\n`
  return new Response(content, { headers: { "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "no-store" } })
}
