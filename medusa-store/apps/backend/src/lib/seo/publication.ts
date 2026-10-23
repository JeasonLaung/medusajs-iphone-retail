import { XMLParser, XMLValidator } from "fast-xml-parser"
import { parse } from "parse5"
import type SeoModuleService from "../../modules/seo/service"
import { httpUrl, invalid, requestText, settings, siteUrl } from "./core"

export const publicKeys = ["seo.site_url", "seo.site_name", "seo.home_path", "seo.default_title", "seo.default_description", "seo.robots_txt", "seo.llms_txt", "seo.indexnow_enabled", "seo.indexnow_key", "bing.verification_enabled", "bing.site_auth_xml", "analytics.enabled", "ga4.measurement_id", "seo.google_search_console.enabled", "gsc.verification_code"]
export async function publicConfig(service: SeoModuleService, path = "/") {
  const all = await settings(service)
  const [page] = await service.listSeoPages({ path }, { take: 1 })
  // Publish the effective origin too: storefront metadata and backend discovery
  // must share the same fallback when no database override has been saved.
  return { settings: { ...Object.fromEntries(publicKeys.filter(k => all[k] !== undefined).map(k => [k, all[k]])), "seo.site_url": siteUrl(all) }, page: page || null }
}
const array = <T>(value: T | T[] | undefined): T[] => value === undefined ? [] : Array.isArray(value) ? value : [value]
export function sitemapDocument(body: string) {
  if (XMLValidator.validate(body) !== true) throw new Error("seo.invalidSitemap")
  const doc = new XMLParser({ ignoreAttributes: false }).parse(body)
  if (!doc.urlset && !doc.sitemapindex) throw new Error("seo.invalidSitemap")
  return {
    pages: array<any>(doc.urlset?.url).map(item => ({ url: String(item.loc || ""), lastmod: String(item.lastmod || "") })),
    children: array<any>(doc.sitemapindex?.sitemap).map(item => String(item.loc || "")),
  }
}
export async function sitemapPages(service: SeoModuleService) {
  const origin = siteUrl(await settings(service)), queue = [`${origin}/sitemap.xml`], seen = new Set<string>()
  const pages = new Map<string, { url: string; path: string; sitemap: string; lastmod: string }>()
  while (queue.length) {
    const url = queue.shift()!
    if (seen.has(url)) continue
    if (seen.size >= 100) throw new Error("seo.sitemapLimit")
    if (httpUrl(url).origin !== origin) throw new Error("seo.crossOriginSitemap")
    seen.add(url)
    const { response, text } = await requestText(url)
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    const doc = sitemapDocument(text)
    for (const child of doc.children) queue.push(new URL(child, url).href)
    for (const item of doc.pages) {
      const parsed = httpUrl(item.url)
      if (parsed.origin !== origin || parsed.search || parsed.hash) throw new Error("seo.crossOriginSitemap")
      pages.set(parsed.href, { ...item, url: parsed.href, path: parsed.pathname, sitemap: url })
      if (pages.size > 50000) throw new Error("seo.sitemapLimit")
    }
  }
  return [...pages.values()]
}
export function inspectHTML(html: string) {
  const result = { canonical: "", languages: {} as Record<string, string>, jsonld: 0, h1: 0, textLength: 0, title: "", description: "" }
  function visit(node: any, inBody = false, skip = false) {
    const attrs = Object.fromEntries((node.attrs || []).map((a: any) => [a.name, a.value])) as Record<string, string>
    const tag = node.tagName
    if (tag === "link") {
      const rel = (attrs.rel || "").split(/\s+/)
      if (rel.includes("canonical")) result.canonical = attrs.href || ""
      if (rel.includes("alternate") && attrs.hreflang) result.languages[attrs.hreflang] = attrs.href
    }
    if (tag === "meta" && attrs.name === "description") result.description = attrs.content || ""
    if (tag === "title") result.title = (node.childNodes || []).map((n: any) => n.value || "").join("")
    if (tag === "script" && attrs.type === "application/ld+json") {
      try { const data = JSON.parse((node.childNodes || []).map((n: any) => n.value || "").join("")); if (data && typeof data === "object" && (Array.isArray(data) ? data.length : Object.keys(data).length)) result.jsonld++ } catch {}
    }
    const ignored = skip || ["script", "style", "noscript", "template", "svg"].includes(tag)
    if (tag === "h1" && !ignored) result.h1++
    if (node.nodeName === "#text" && inBody && !ignored) result.textLength += (node.value || "").trim().length
    for (const child of node.childNodes || []) visit(child, inBody || tag === "body", ignored)
  }
  visit(parse(html))
  return result
}
export function validBingXML(value: string) {
  if (value.length > 65536 || XMLValidator.validate(value) !== true) return false
  const doc = new XMLParser().parse(value)
  return array<any>(doc.users?.user).some(v => typeof v === "string" && !!v.trim())
}
export async function siteAudit(service: SeoModuleService) {
  const values = await settings(service), site = siteUrl(values), home = values["seo.home_path"] || "/dk-en"
  const checks: { key: string; category: string; passed: boolean; detail: string; url: string }[] = []
  async function check(key: string, category: string, url: string, test: (body: string) => boolean) {
    try {
      const { response, text } = await requestText(url)
      checks.push({ key, category, passed: response.status === 200 && test(text), detail: `HTTP ${response.status}`, url })
    } catch { checks.push({ key, category, passed: false, detail: "seo.auditFetchFailed", url }) }
  }
  await Promise.all([
    check("robots", "SEO", `${site}/robots.txt`, text => /^User-agent:/im.test(text)),
    check("sitemap", "SEO", `${site}/sitemap.xml`, text => { sitemapDocument(text); return true }),
    check("llms", "GEO", `${site}/llms.txt`, text => /^#\s+\S/m.test(text)),
  ])
  const url = new URL(home, site).href
  await check("canonical_hreflang", "SEO", url, text => {
    const signals = inspectHTML(text)
    return signals.canonical.replace(/\/$/, "") === url.replace(/\/$/, "") && !!signals.languages["x-default"] && Object.keys(signals.languages).length > 1
  })
  await check("structured_data", "SEO", url, text => inspectHTML(text).jsonld > 0)
  await check("crawlable_content", "GEO", url, text => { const signals = inspectHTML(text); return signals.h1 > 0 && signals.textLength >= 200 })
  await check("metadata", "SEO", url, text => { const signals = inspectHTML(text); return !!signals.title && !!signals.description })
  return { site_url: site, checked_at: new Date().toISOString(), checks }
}
export function validateIndexURLs(urls: string[], origin: string) {
  if (!urls.length || urls.length > 10000) return invalid("indexNowUrlCount")
  return [...new Set(urls.map(value => {
    const url = httpUrl(value)
    if (url.origin !== origin || url.search || url.hash) return invalid("indexNowSameSite")
    return url.href
  }))]
}
export async function submitIndexNow(service: SeoModuleService, requested?: string[]) {
  const values = await settings(service), origin = siteUrl(values), key = values["seo.indexnow_key"]
  if (values["seo.indexnow_enabled"] !== "true" || !/^[A-Za-z0-9-]{8,128}$/.test(key || "")) return invalid("indexNowRequired")
  const urls = validateIndexURLs(requested ?? (await sitemapPages(service)).map(p => p.url), origin)
  const base = { submitted_count: urls.length, urls: { list: urls }, submitted_at: new Date() }
  try {
    // Verify the exact public key resource before submitting URLs.
    const verification = await requestText(`${origin}/${key}.txt`)
    if (verification.response.status !== 200 || verification.text.trim() !== key) throw new Error("seo.indexNowVerificationFailed")
    const result = await requestText("https://api.indexnow.org/indexnow", { method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ host: new URL(origin).host, key, keyLocation: `${origin}/${key}.txt`, urlList: urls }) })
    const success = [200, 202].includes(result.response.status)
    const saved = await service.createSeoSubmissions({ ...base, success, http_status: result.response.status,
      response_headers: Object.fromEntries(result.response.headers), response_body: result.text, error: success ? "" : `HTTP ${result.response.status}` })
    if (success) {
      const [existing] = await service.listSeoSettings({ key: "seo.indexnow_last_submitted_at" })
      const update = { key: "seo.indexnow_last_submitted_at", value: base.submitted_at.toISOString(), group: "indexnow" }
      if (existing) await service.updateSeoSettings({ id: existing.id, ...update }); else await service.createSeoSettings(update)
    }
    return saved
  } catch (error) {
    return service.createSeoSubmissions({ ...base, success: false, error: error instanceof Error && error.message.startsWith("seo.") ? error.message : "seo.indexNowFailed" })
  }
}
