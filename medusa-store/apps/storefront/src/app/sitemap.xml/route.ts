import { sdk } from "@lib/config"
import { getSEO, seoOrigin } from "@lib/data/seo"
import { getHomeCopy } from "@modules/home/i18n"
import { variantSlug } from "@lib/util/product-variants"

export const dynamic = "force-dynamic"
const xml = (text: string) => text.replace(/[<>&"']/g, c => ({ "<": "&lt;", ">": "&gt;", "&": "&amp;", '"': "&quot;", "'": "&apos;" })[c]!)
export async function GET(request: Request) {
  const [{ settings }, { regions }, { pages }] = await Promise.all([getSEO(), sdk.store.region.list({ limit: 100 }, { cache: "no-store" }), sdk.client.fetch<{ pages: { path: string; robots: string; canonical: string; hreflang: Record<string, string>; updated_at: string }[] }>("/store/seo/pages", { cache: "no-store" })])
  const overrides = new Map(pages.map(page => [page.path, page]))
  const origin = seoOrigin(settings), entries = new Map<string, { lastmod?: string; country: string; rest: string }>()
  const countries = Array.from(new Set(regions.flatMap(r => r.countries?.map(c => c.iso_2).filter((c): c is string => !!c) || [])))
  const products = []; let offset = 0
  for (;;) {
    const result = await sdk.store.product.list({ limit: 100, offset, fields: "id,handle,updated_at,*variants" }, { cache: "no-store" })
    products.push(...result.products); offset += result.products.length
    if (offset >= result.count || !result.products.length) break
  }
  const collections = []; offset = 0
  for (;;) {
    const result = await sdk.store.collection.list({ limit: 100, offset }, { cache: "no-store" })
    collections.push(...result.collections); offset += result.collections.length
    if (offset >= result.count || !result.collections.length) break
  }
  for (const country of countries) {
    for (const language of ["en", "zh"]) {
      const prefix = `/${country}-${language}`
      for (const rest of ["", "/store", "/about", "/help", "/reviews", "/blog", "/sell", ...getHomeCopy(language === "zh" ? "zh-CN" : "en").blogPosts.map(p => `/blog/${p.slug}`)]) entries.set(`${prefix}${rest}`, { country, rest })
      for (const product of products) {
        if (!product.handle) continue
        const slugs = [product.handle, ...(product.variants || []).map(variantSlug).filter((slug): slug is string => !!slug)]
        for (const slug of slugs) { const rest = `/products/${encodeURIComponent(slug)}`; entries.set(`${prefix}${rest}`, { lastmod: product.updated_at || undefined, country, rest }) }
      }
      for (const collection of collections) if (collection.handle) { const rest = `/collections/${encodeURIComponent(collection.handle)}`; entries.set(`${prefix}${rest}`, { lastmod: collection.updated_at || undefined, country, rest }) }
    }
  }
  // Respect page-level noindex and external canonicals without exposing management data publicly.
  const documents = Array.from(entries).map(([path, entry]) => {
    const page = overrides.get(path)
    if (page?.robots?.split(",").some(r => r.trim() === "noindex") || (page?.canonical && page.canonical.replace(/\/$/, "") !== `${origin}${path}`)) return ""
    const lastmod = [entry.lastmod, page?.updated_at].filter(Boolean).sort().at(-1)
    const alternates = page?.hreflang && Object.keys(page.hreflang).length ? Object.entries(page.hreflang) : ["en", "zh-CN", "x-default"].map(lang => [lang, `${origin}/${entry.country}-${lang === "zh-CN" ? "zh" : "en"}${entry.rest}`])
    return `<url><loc>${xml(`${origin}${path}`)}</loc>${lastmod ? `<lastmod>${xml(new Date(lastmod).toISOString())}</lastmod>` : ""}${alternates.map(([lang, href]) => `<xhtml:link rel="alternate" hreflang="${xml(lang)}" href="${xml(href)}"/>`).join("")}</url>`
  })
  const filtered = documents.filter(Boolean), size = 1000, part = new URL(request.url).searchParams.get("part")
  let content: string
  if (part === null && filtered.length > size) {
    content = `<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${Array.from({ length: Math.ceil(filtered.length / size) }, (_, i) => `<sitemap><loc>${xml(`${origin}/sitemap.xml?part=${i + 1}`)}</loc></sitemap>`).join("")}</sitemapindex>`
  } else {
    const index = part === null ? 1 : Number(part)
    if (!Number.isInteger(index) || index < 1 || (index - 1) * size >= filtered.length) return new Response(null, { status: 404 })
    content = `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">${filtered.slice((index - 1) * size, index * size).join("")}</urlset>`
  }
  return new Response(`<?xml version="1.0" encoding="UTF-8"?>${content}`, { headers: { "Content-Type": "application/xml; charset=utf-8", "Cache-Control": "no-store" } })
}
