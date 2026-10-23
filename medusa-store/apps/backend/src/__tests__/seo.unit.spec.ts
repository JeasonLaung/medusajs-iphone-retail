import { classifyTraffic, parseLogLine } from "../lib/seo/ingestion"
import { parseCitationResponse, providerRequest, runCitation } from "../lib/seo/citations"
import { decrypt, encrypt, normalizePath, safeRecord, settings } from "../lib/seo/core"
import { inspectHTML, publicConfig, sitemapDocument, validateIndexURLs, validBingXML } from "../lib/seo/publication"
import { pendingIndexPages, seoCommand } from "../lib/seo/operations"
import { visitFilters, visitStats } from "../lib/seo/analytics"
import { googleOverview, testGoogle } from "../lib/seo/google"

const log = (target: string, agent = "Mozilla/5.0", referrer = "https://chatgpt.com/") => `127.0.0.1 - - [23/Oct/2026:09:05:06 +0800] "GET ${target} HTTP/1.1" 200 100 "${referrer}" "${agent}"`
describe("SEO/GEO attribution", () => {
  it("uses compatible UTM with click IDs, without letting unrelated UTM override the ad platform", () => {
    expect(classifyTraffic(new URL("https://shop.test/dk-en?fbclid=id&utm_source=instagram&utm_medium=social"), "", "shop.test")).toMatchObject({ source: "instagram", platform: "meta_ads", matched_by: "click_id+utm" })
    expect(classifyTraffic(new URL("https://shop.test/dk-en?gclid=id&utm_source=facebook"), "", "shop.test")).toMatchObject({ source: "google", platform: "google_ads", matched_by: "click_id" })
  })
  it("excludes internal referrers, assets, failed requests and non-AI bots from human acquisition", () => {
    expect(parseLogLine(log("/dk-en", "Mozilla/5.0", "https://shop.test/cart"), "source", "shop.test")).toBeNull()
    expect(parseLogLine(log("/_next/a.js?utm_source=google"), "source", "shop.test")).toBeNull()
    expect(parseLogLine(log("/dk-en", "MonitoringBot"), "source", "shop.test")).toBeNull()
    expect(parseLogLine(log("/dk-en").replace(' 200 100 ', ' 404 100 '), "source", "shop.test")).toBeNull()
  })
  it("records real log timestamps, AI referrals and masked crawler IPs", () => {
    const human = parseLogLine(log("/dk-en/store?utm_campaign=October"), "source", "shop.test")
    expect(human).toMatchObject({ kind: "traffic", value: { source: "chatgpt", platform: "ai_referral", landing_path: "/dk-en/store", campaign: "October" } })
    expect(human?.value.occurred_at.toISOString()).toBe("2026-10-23T01:05:06.000Z")
    const crawler = parseLogLine(log("/dk-en", "OAI-SearchBot"), "source", "shop.test")
    expect(crawler).toMatchObject({ kind: "crawler", value: { crawler_name: "OAI-SearchBot" } })
    expect(crawler?.value).not.toHaveProperty("client_ip")
    expect(crawler?.value.client_ip_hash).toHaveLength(64)
  })
  it("paginates reference path rankings by ten without losing aggregate totals", async () => {
    const records = Array.from({ length: 25 }, (_, i) => ({ landing_path: `/dk-en/page-${String(i).padStart(2, "0")}`, source: "chatgpt", platform: "ai_referral", client_ip: "one", location: "DK", occurred_at: "2026-10-21T01:00Z" }))
    const result = await visitStats({ listSeoTrafficVisits: jest.fn().mockResolvedValue(records) } as any, { path_page: 2, path_size: 10 }, true)
    expect(result).toMatchObject({ total_count: 25, top_paths_total: 25, top_sources: [{ name: "chatgpt", count: 25 }] })
    expect(result.top_paths).toHaveLength(10)
    expect(result.top_paths[0].name).toBe("/dk-en/page-10")
    expect(result.top_paths[9].name).toBe("/dk-en/page-19")
  })
  it("groups every matching path, retains source breakdown and sums daily unique IPs", async () => {
    const records = [
      { landing_path: "/dk-en", source: "chatgpt", platform: "ai_referral", client_ip: "one", location: "GB", occurred_at: "2026-10-21T01:00Z" },
      { landing_path: "/dk-en", source: "chatgpt", platform: "ai_referral", client_ip: "one", location: "GB", occurred_at: "2026-10-21T02:00Z" },
      { landing_path: "/dk-en", source: "google", platform: "google", client_ip: "one", location: "GB", occurred_at: "2026-10-22T01:00Z" },
    ]
    const result = await visitStats({ listSeoTrafficVisits: jest.fn().mockResolvedValue(records) } as any, {}, true)
    expect(result).toMatchObject({ total_count: 3, top_paths: [{ name: "/dk-en", count: 3, daily_unique_ip_count: 2, sources: [{ name: "chatgpt", count: 2 }, { name: "google", count: 1 }] }] })
    expect(() => visitFilters({ start: "bad" }, true)).toThrow("seo.invalidDate")
    expect(visitFilters({ locale: "en", path: "iphone" }, true)).toEqual({ $and: [{ landing_path: { $like: "%iphone%" } }, { $or: [{ landing_path: { $like: "/__-en/%" } }, { landing_path: { $like: "/__-en" } }] }] })
  })
})
describe("SEO publication and secrets", () => {
  it("publishes the effective origin so storefront sitemap and backend discovery agree", async () => {
    const previous = process.env.STOREFRONT_URL
    try {
      process.env.STOREFRONT_URL = "https://shop.test"
      const service = { listSeoSettings: async () => [], listSeoPages: async () => [] } as any
      expect((await publicConfig(service)).settings["seo.site_url"]).toBe("https://shop.test")
      service.listSeoSettings = async () => [{ key: "seo.site_url", value: "https://override.test/" }]
      expect((await publicConfig(service)).settings["seo.site_url"]).toBe("https://override.test")
    } finally {
      if (previous === undefined) delete process.env.STOREFRONT_URL; else process.env.STOREFRONT_URL = previous
    }
  })
  it("retains unsubmitted URLs across IndexNow batches and manual subsets", () => {
    const pages = [{ url: "https://shop.test/a", lastmod: "2026-10-21" }, { url: "https://shop.test/b", lastmod: "" }, { url: "https://shop.test/c", lastmod: "2026-10-23" }]
    expect(pendingIndexPages(pages, [{ submitted_at: "2026-10-22", urls: { list: [pages[0].url, pages[2].url] } }])).toEqual([pages[1], pages[2]])
  })
  beforeEach(() => { process.env.SEO_ENCRYPTION_KEY = "seo-test-key-only" })
  afterEach(() => { delete process.env.SEO_ENCRYPTION_KEY; jest.restoreAllMocks() })
  it("encrypts authenticated secrets and redacts them on every read / save response", () => {
    const value = encrypt("private-test-value")
    expect(value).not.toContain("private-test-value")
    expect(decrypt(value)).toBe("private-test-value")
    expect(safeRecord("settings", { value, is_secret: true }).value).toBe("********")
    expect(safeRecord("providers", { api_key: value })).toEqual({ api_key_configured: true })
  })
  it("retains blank secrets, requires explicit clearing and never returns ciphertext", async () => {
    const value = encrypt("stored-value"), service = { listSeoSettings: jest.fn().mockResolvedValue([{ id: "s", key: "gsc.oauth.client_secret", value, is_secret: true }]), updateSeoSettings: jest.fn().mockImplementation(v => v) }
    const container = { resolve: () => service } as any
    const response = await seoCommand(container, { action: "save", resource: "settings", data: { key: "gsc.oauth.client_secret", value: "" } })
    expect(service.updateSeoSettings).toHaveBeenCalledWith(expect.objectContaining({ value, is_secret: true }))
    expect(response.value).toBe("********")
    await seoCommand(container, { action: "save", resource: "settings", data: { key: "gsc.oauth.client_secret", value: "", clear_secret: true } })
    expect(service.updateSeoSettings).toHaveBeenLastCalledWith(expect.objectContaining({ value: "" }))
    expect(await settings(service as any)).toEqual({})
  })
  it("parses sitemap indexes and leaves, rejects invalid XML and cross-site submissions", () => {
    expect(sitemapDocument('<sitemapindex><sitemap><loc>https://shop.test/sitemap.xml?part=1&amp;x=2</loc></sitemap></sitemapindex>').children).toEqual(["https://shop.test/sitemap.xml?part=1&x=2"])
    expect(sitemapDocument('<urlset><url><loc>https://shop.test/dk-en</loc><lastmod>2026-10-23</lastmod></url></urlset>').pages[0].lastmod).toBe("2026-10-23")
    expect(() => sitemapDocument('<urlset><url></urlset>')).toThrow()
    expect(() => validateIndexURLs(["https://other.test/a"], "https://shop.test")).toThrow("seo.indexNowSameSite")
    expect(() => validateIndexURLs(["https://shop.test/a?x=1"], "https://shop.test")).toThrow("seo.indexNowSameSite")
    expect(validateIndexURLs(["https://shop.test/a", "https://shop.test/a"], "https://shop.test")).toEqual(["https://shop.test/a"])
  })
  it("checks actual HTML signals while excluding script payloads from crawlable body text", () => {
    const html = `<html><head><title>Phone</title><meta name="description" content="Refurbished"><link rel="canonical" href="https://shop.test/dk-en"><link rel="alternate" hreflang="x-default" href="https://shop.test/dk-en"><script type="application/ld+json">{"@type":"WebSite"}</script></head><body><h1>Phone</h1><p>Real text</p><script>${"script ".repeat(200)}</script></body></html>`
    expect(inspectHTML(html)).toMatchObject({ jsonld: 1, h1: 1, title: "Phone", description: "Refurbished", canonical: "https://shop.test/dk-en", textLength: 14 })
    expect(inspectHTML('<script type="application/ld+json">not json</script>').jsonld).toBe(0)
    expect(validBingXML('<users><user>verification</user></users>')).toBe(true)
    expect(validBingXML('<users><user></users>')).toBe(false)
    expect(() => normalizePath("//other.test")).toThrow()
  })
})
describe("GEO provider formats and evidence", () => {
  it("supports standard Responses, standard messages and the old input gateway explicitly", () => {
    const provider = { provider: "chatgpt", model: "configured-model", base_url: "https://provider.test/v1" }
    expect(providerRequest({ ...provider, wire_api: "responses" }, "question")).toMatchObject({ url: "https://provider.test/v1/responses", body: { input: "question", tools: [{ type: "web_search" }] } })
    expect(providerRequest({ ...provider, wire_api: "chat" }, "question").body).toMatchObject({ messages: [{ role: "user", content: "question" }] })
    expect(providerRequest({ ...provider, wire_api: "chat_input" }, "question").body).toMatchObject({ input: "question" })
  })
  it("keeps provider annotations distinct from extracted plain links, without duplicate answer text", () => {
    const parsed = parseCitationResponse({ output_text: "duplicate", output: [{ content: [{ text: "EuroRefurb https://shop.test/phones https://other.test/help.", annotations: [{ type: "url_citation", url: "https://shop.test/phones", title: "Phones" }] }] }] })
    expect(parsed.answer).not.toContain("duplicate")
    expect(parsed.evidence).toEqual([{ source_url: "https://shop.test/phones", source_title: "Phones", evidence_type: "provider_citation" }, { source_url: "https://other.test/help", source_title: "", evidence_type: "answer_link" }])
    expect(() => parseCitationResponse({ choices: [] })).toThrow("seo.emptyAnswer")
  })
  it("persists provider failures and never stores provider response secrets", async () => {
    process.env.SEO_ENCRYPTION_KEY = "seo-test-key-only"
    const service = {
      retrieveSeoMonitor: async () => ({ id: "m", provider: "chatgpt", query_text: "question", brand_name: "EuroRefurb" }), listSeoRuns: async () => [],
      createSeoRuns: jest.fn().mockResolvedValue({ id: "run" }), updateSeoRuns: jest.fn().mockImplementation(v => v), updateSeoMonitors: jest.fn(),
      listSeoProviders: async () => [{ api_key: encrypt("fake-key"), provider: "chatgpt", wire_api: "responses", base_url: "https://provider.test/v1", model: "test" }],
    }
    jest.spyOn(global, "fetch").mockResolvedValue(new Response("secret provider body", { status: 401 }))
    const container = { resolve: (key: string) => key === "locking" ? { execute: async (_key: string, fn: () => any) => fn() } : service } as any
    expect(await runCitation(container, "m")).toMatchObject({ status: "failed", error_message: "HTTP 401" })
    expect(JSON.stringify(service.updateSeoRuns.mock.calls)).not.toContain("secret provider body")
    delete process.env.SEO_ENCRYPTION_KEY; jest.restoreAllMocks()
  })
})
describe("Google reporting", () => {
  afterEach(() => jest.restoreAllMocks())
  it("reports GA4 and GSC independently, with weighted Search Console position", async () => {
    const rows = ["ga4", "gsc"].flatMap(p => ["client_id", "client_secret", "refresh_token"].map(k => ({ key: `${p}.oauth.${k}`, value: "test", is_secret: false })))
    rows.push({ key: "ga4.property_id", value: "123", is_secret: false }, { key: "gsc.resource", value: "sc-domain:shop.test", is_secret: false })
    const service = { listSeoSettings: async () => rows } as any
    jest.spyOn(global, "fetch").mockImplementation(async (input: any) => {
      const url = String(input)
      if (url.includes("oauth2")) return Response.json({ access_token: "private-token" })
      if (url.includes("analyticsdata")) return new Response("no permission", { status: 403 })
      return Response.json({ rows: [{ keys: ["2026-10-20"], clicks: 2, impressions: 10, ctr: .2, position: 2 }, { keys: ["2026-10-21"], clicks: 3, impressions: 30, ctr: .1, position: 6 }] })
    })
    const result = await googleOverview(service)
    expect(result.ga4).toMatchObject({ connected: false })
    expect(result.gsc).toMatchObject({ connected: true, clicks: 5, impressions: 40, ctr: .125, position: 5 })
    expect(JSON.stringify(result)).not.toContain("private-token")
    await expect(testGoogle(service, "ga4")).rejects.toThrow("HTTP 403")
  })
})
