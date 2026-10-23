import type { MedusaContainer } from "@medusajs/framework/types"
import { allRows, encrypt, httpUrl, invalid, jsonValue, normalizePath, resources, safeRecord, secretSetting, seoService, settings, type Resource } from "./core"
import { allowedLogPath, scanSources } from "./ingestion"
import { runCitation } from "./citations"
import { submitIndexNow, validBingXML, sitemapPages } from "./publication"
import { testGoogle } from "./google"

export type SeoCommand = { action: "save" | "delete" | "scan" | "run" | "indexnow" | "google-test" | "tick"; resource?: Resource; id?: string; data?: Record<string, any> }
const writable = new Set<Resource>(["settings", "page-metas", "crawler-sources", "providers", "citation-monitors"])
const text = (value: unknown, max = 20000) => typeof value === "string" ? value.trim().slice(0, max) : ""
export async function seoCommand(container: MedusaContainer, input: SeoCommand): Promise<any> {
  const service = seoService(container), data = input.data || {}
  if (input.action === "scan") return scanSources(container)
  if (input.action === "run") return runCitation(container, input.id || "")
  if (input.action === "indexnow") return container.resolve("locking").execute("seo-indexnow", () => submitIndexNow(service, data.urls), { timeout: 5, expire: 300 })
  if (input.action === "google-test") return testGoogle(service, text(data.provider))
  if (input.action === "tick") return scheduledTick(container)
  const resource = input.resource
  if (!resource || !writable.has(resource)) return invalid("invalidResource")
  const suffix = resources[resource], dynamic = service as any
  if (input.action === "delete") {
    if (!input.id) return invalid("idRequired")
    if (resource === "citation-monitors") {
      const running = await service.listSeoRuns({ monitor_id: input.id, status: "running" }, { take: 1 })
      if (running.length) return invalid("monitorRunning")
    }
    await dynamic[`delete${suffix}`]([input.id])
    return { id: input.id, deleted: true }
  }
  let values: Record<string, any> = {}
  if (resource === "settings") {
    const key = text(data.key, 128)
    if (!/^[a-z][a-z0-9_.-]{0,127}$/.test(key)) return invalid("invalidSettingKey")
    if (["seo.indexnow_last_submitted_at", "seo.indexnow_last_checked_at"].includes(key)) return invalid("readOnlySetting")
    let value = text(data.value, 65536)
    const isSecret = secretSetting(key) || data.is_secret === true
    const [existing] = await service.listSeoSettings({ key })
    if (input.id && existing && existing.id !== input.id) return invalid("duplicateSetting")
    input.id ||= existing?.id
    if (key.endsWith("enabled") || key === "seo.indexnow_auto_submit") { if (!["true", "false"].includes(value)) return invalid("invalidBoolean") }
    if (key === "seo.site_url") { const url = httpUrl(value); if (url.pathname !== "/" || url.search || url.hash) return invalid("invalidUrl"); value = url.origin }
    if (key === "seo.home_path") value = normalizePath(value)
    if (key === "seo.indexnow_key" && value && !/^[A-Za-z0-9-]{8,128}$/.test(value)) return invalid("invalidIndexNowKey")
    if (key === "bing.site_auth_xml" && value && !validBingXML(value)) return invalid("invalidBingXml")
    if (key === "ga4.measurement_id" && value && !/^G-[A-Z0-9]+$/.test(value)) return invalid("invalidMeasurementId")
    if (key === "gsc.verification_code" && /[<>"']/.test(value)) return invalid("invalidVerificationCode")
    if (isSecret) {
      if (value === "********") value = ""
      value = data.clear_secret === true ? "" : value ? encrypt(value) : existing?.value || ""
    } else if (existing?.is_secret && !data.clear_secret) return invalid("cannotExposeSecret")
    values = { key, value, is_secret: isSecret, group: text(data.group, 32) || "site" }
  }
  if (resource === "page-metas") {
    const path = normalizePath(text(data.path, 2048)), hreflang = jsonValue(data.hreflang, {}), structured = jsonValue(data.structured_data, [])
    if (!hreflang || Array.isArray(hreflang) || typeof hreflang !== "object") return invalid("invalidHreflang")
    for (const [language, url] of Object.entries(hreflang)) { if (!/^[a-z]{2}(?:-[A-Z]{2})?$|^x-default$/.test(language) || typeof url !== "string") return invalid("invalidHreflang"); httpUrl(url) }
    if (!structured || typeof structured !== "object") return invalid("invalidStructuredData")
    const canonical = text(data.canonical, 2048), og_image = text(data.og_image, 2048)
    if (canonical) httpUrl(canonical)
    if (og_image) httpUrl(og_image)
    const robots = text(data.robots, 128) || "index,follow"
    if (robots.split(",").some(v => !["index", "noindex", "follow", "nofollow", "noarchive", "nosnippet", "noimageindex"].includes(v.trim()))) return invalid("invalidRobots")
    values = { path, title: text(data.title, 512), description: text(data.description, 4000), canonical, og_image, robots, hreflang, structured_data: structured }
  }
  if (resource === "providers") {
    const provider = text(data.provider), model = text(data.model, 128), wire_api = text(data.wire_api) || (provider === "chatgpt" ? "responses" : "chat")
    if (!["chatgpt", "deepseek"].includes(provider) || !["responses", "chat", "chat_input"].includes(wire_api) || !model) return invalid("invalidProvider")
    const [existing] = await service.listSeoProviders({ provider: provider as any })
    if (input.id && existing && existing.id !== input.id) return invalid("duplicateProvider")
    input.id ||= existing?.id
    const base_url = httpUrl(text(data.base_url) || (provider === "chatgpt" ? "https://api.openai.com/v1" : "https://api.deepseek.com")).href.replace(/\/$/, "")
    if (base_url.includes("?")) return invalid("invalidUrl")
    values = { provider, base_url, model, wire_api, api_key: data.clear_secret ? "" : text(data.api_key) ? encrypt(text(data.api_key)) : existing?.api_key || "" }
  }
  if (resource === "crawler-sources") {
    const interval = Number(data.interval_minutes ?? 10)
    if (!Number.isInteger(interval) || interval < 1 || interval > 10080 || !text(data.source_name)) return invalid("invalidInterval")
    values = { source_name: text(data.source_name, 128), file_path: allowedLogPath(text(data.file_path, 4096)), source_type: "nginx_file", interval_minutes: interval,
      enabled: data.enabled !== false, collect_ai_crawlers: data.collect_ai_crawlers !== false, collect_traffic: data.collect_traffic !== false }
    if (input.id) {
      const row = await service.retrieveSeoSource(input.id)
      if (row.file_path !== values.file_path) Object.assign(values, { read_offset: 0, read_lines: 0, file_fingerprint: "", last_scanned_at: null, last_error: "" })
    }
  }
  if (resource === "citation-monitors") {
    const interval = Number(data.interval_minutes ?? 1440), provider = text(data.provider)
    if (!Number.isInteger(interval) || interval < 1 || interval > 525600) return invalid("invalidInterval")
    if (!["chatgpt", "deepseek"].includes(provider)) return invalid("invalidProvider")
    values = { monitor_name: text(data.monitor_name, 128), brand_name: text(data.brand_name, 128), provider, query_text: text(data.query_text, 20000), interval_minutes: interval, enabled: data.enabled === true }
    if (!values.monitor_name || !values.brand_name || !values.query_text) return invalid("requiredFields")
  }
  const row = input.id ? await dynamic[`update${suffix}`]({ id: input.id, ...values }) : await dynamic[`create${suffix}`](values)
  return safeRecord(resource, row)
}
async function scheduledTick(container: MedusaContainer) {
  const service = seoService(container)
  await scanSources(container, false)
  for (const monitor of await allRows(service, "citation-monitors", { enabled: true })) {
    if (!monitor.last_run_at || Date.now() - new Date(monitor.last_run_at).valueOf() >= monitor.interval_minutes * 60000) {
      try { await runCitation(container, monitor.id) } catch { /* Another worker may own this monitor. */ }
    }
  }
  const values = await settings(service)
  if (values["seo.indexnow_enabled"] !== "true" || values["seo.indexnow_auto_submit"] !== "true") return
  await container.resolve("locking").execute("seo-indexnow", async () => {
    const [last] = await service.listSeoSubmissions({}, { take: 1, order: { submitted_at: "DESC" } })
    // Limit repeated failures to one attempt per hour, and successful resubmissions to changed content.
    if (last && Date.now() - new Date(last.submitted_at).valueOf() < 3600000) return
    const [checked] = await service.listSeoSettings({ key: "seo.indexnow_last_checked_at" })
    if (checked && Date.now() - new Date(checked.value).valueOf() < 3600000) return
    const stamp = { key: "seo.indexnow_last_checked_at", value: new Date().toISOString(), group: "indexnow" }
    if (checked) await service.updateSeoSettings({ id: checked.id, ...stamp }); else await service.createSeoSettings(stamp)
    const pages = await sitemapPages(service)
    const changed = pendingIndexPages(pages, await allRows(service, "submissions", { success: true }))
    if (changed.length) await submitIndexNow(service, changed.slice(0, 10000).map(p => p.url))
  }, { timeout: 5, expire: 300 })
}

export function pendingIndexPages<T extends { url: string; lastmod: string }>(pages: T[], submissions: any[]): T[] {
  const submitted = new Map<string, number>()
  for (const row of submissions) {
    const timestamp = new Date(row.submitted_at).valueOf()
    for (const url of row.urls?.list || []) submitted.set(url, Math.max(submitted.get(url) || 0, timestamp))
  }
  // Track acknowledgement per URL: manual subsets and 10,000-URL batches must not hide the remaining pages.
  return pages.filter(page => !submitted.has(page.url) || (!!page.lastmod && new Date(page.lastmod).valueOf() > submitted.get(page.url)!))
}
