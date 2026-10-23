import "server-only"
import { cache } from "react"
import { headers } from "next/headers"
import type { Metadata } from "next"
import { sdk } from "@lib/config"
import { getBaseURL } from "@lib/util/env"

type SEOPage = { path: string; title: string; description: string; canonical: string; og_image: string; hreflang: Record<string, string> | null; structured_data: object | object[] | null; robots: string }
export type SEOConfig = { settings: Record<string, string>; page: SEOPage | null }
export const getSEO = cache(async (path = "/"): Promise<SEOConfig> => {
  return sdk.client.fetch<SEOConfig>("/store/seo", { query: { path: path.replace(/\/$/, "") || "/" }, cache: "no-store" })
})
export const seoOrigin = (values: Record<string, string>) => (values["seo.site_url"] || getBaseURL()).replace(/\/$/, "")
export async function currentSEO() {
  const h = await headers(), raw = h.get("x-storefront-url")
  const path = raw ? new URL(raw).pathname.replace(/\/$/, "") || "/" : "/"
  // Commerce remains available during an SEO service outage; defaults stay intact.
  let config: SEOConfig
  try { config = await getSEO(path) } catch { config = { settings: {}, page: null } }
  return { ...config, path }
}
function privatePage(path: string) {
  return /\/(account|checkout|cart|order|payment-result|verify-account)(\/|$)/.test(path) || /\/sell\/(iphone|requests)(\/|$)/.test(path)
}
export async function withSEO(defaults: Metadata = {}): Promise<Metadata> {
  const { settings: values, page, path } = await currentSEO(), origin = seoOrigin(values)
  const canonical = page?.canonical || `${origin}${path}`
  const segment = path.split("/")[1], country = segment?.match(/^([a-z]{2})-(en|zh)$/)?.[1]
  const languages = country ? { en: `${origin}${path.replace(/^\/[a-z]{2}-(en|zh)/, `/${country}-en`)}`, "zh-CN": `${origin}${path.replace(/^\/[a-z]{2}-(en|zh)/, `/${country}-zh`)}`, "x-default": `${origin}${path.replace(/^\/[a-z]{2}-(en|zh)/, `/${country}-en`)}` } : undefined
  const title = page?.title || defaults.title || values["seo.default_title"] || values["seo.site_name"] || "EuroRefurb"
  const description = page?.description || defaults.description || values["seo.default_description"] || undefined
  const robots = page?.robots?.split(",").map(v => v.trim())
  return {
    ...defaults, metadataBase: new URL(origin), title: page?.title ? { absolute: page.title } : title, description,
    alternates: { ...defaults.alternates, canonical, languages: page?.hreflang && Object.keys(page.hreflang).length ? page.hreflang : languages },
    robots: privatePage(path) ? { index: false, follow: false } : page ? { index: !robots?.includes("noindex"), follow: !robots?.includes("nofollow"), noarchive: robots?.includes("noarchive"), nosnippet: robots?.includes("nosnippet"), noimageindex: robots?.includes("noimageindex") } : defaults.robots,
    openGraph: { ...defaults.openGraph, title: typeof title === "string" ? title : page?.title || undefined, description, url: canonical, ...(page?.og_image ? { images: [page.og_image] } : {}) },
    ...(values["seo.google_search_console.enabled"] === "true" && values["gsc.verification_code"] ? { verification: { ...defaults.verification, google: values["gsc.verification_code"] } } : {}),
  }
}
export const jsonLD = (value: unknown) => JSON.stringify(value).replace(/</g, "\\u003c").replace(/>/g, "\\u003e").replace(/&/g, "\\u0026")
