import { getI18n } from "react-i18next"
import { sdk } from "../../lib/sdk"

// Adapt the reference UI's envelopes, Unix timestamps and JSON editor values to Medusa.
// All HTTP calls retain the project's SDK session authentication.
export type ApiEnvelope<T> = { code: number; message: string; data: T }
export type PageResult<T> = { list: T[]; total: number; page: number; size: number }
const prefix = "/operations/seo/"
const auditTitles: Record<string, string> = { robots: "robots.txt", sitemap: "XML Sitemap", llms: "llms.txt", canonical_hreflang: "Canonical / Hreflang", structured_data: "JSON-LD 结构化数据", crawlable_content: "服务端可抓取正文", metadata: "标题与描述" }
function localized(value: string) { return value.startsWith("seo.") ? String(getI18n().t(value, { defaultValue: value })) : value }
function normalize(row: any): any {
  if (!row || typeof row !== "object") return row
  if (Array.isArray(row)) return row.map(normalize)
  const next = { ...row }
  for (const [key, value] of Object.entries(next)) {
    if (["created_at", "occurred_at", "last_scanned_at", "last_run_at", "started_at", "finished_at"].includes(key)) next[key] = value ? Math.floor(new Date(value as string).valueOf() / 1000) : 0
    if (["error", "error_message", "last_error", "detail"].includes(key) && typeof value === "string") next[key] = localized(value)
  }
  if ("traffic_matched_lines" in next) next.traffic_read_lines = next.read_lines
  if ("hreflang" in next) next.hreflang = JSON.stringify(next.hreflang || {}, null, 2)
  if ("structured_data" in next) next.structured_data = JSON.stringify(next.structured_data || [], null, 2)
  return next
}
async function read<T = any>(resource: string, query: Record<string, any> = {}): Promise<T> {
  return sdk.client.fetch<T>(`/admin/seo/${resource}`, { query })
}
async function all(resource: string, query: Record<string, any> = {}) {
  const rows: any[] = []
  for (let page = 1; ; page++) {
    const result = await read<PageResult<any>>(resource, { ...query, page, size: 200 })
    rows.push(...result.list)
    if (rows.length >= result.total || !result.list.length) return rows.map(normalize)
  }
}
async function action(action: string, resource?: string, id?: string, data?: any) {
  const response = await sdk.client.fetch<{ result: any }>("/admin/seo/commands", { method: "POST", body: { action, resource, id, data } })
  return normalize(response.result)
}
function location(path: string, params: Record<string, any> = {}) {
  const url = new URL(path, "http://reference.local")
  const parts = url.pathname.replace(prefix, "").split("/").map(decodeURIComponent)
  const query = { ...Object.fromEntries(url.searchParams), ...params }
  return { resource: parts[0], id: parts[1], tail: parts[2], query }
}
async function execute(method: string, path: string, body?: any, params?: Record<string, any>): Promise<any> {
  const { resource, id, query } = location(path, params)
  if (method === "get") {
    if (resource === "citation-runs" && id) {
      const result = await read<PageResult<any>>(resource, { id })
      if (!result.list[0]) throw new Error("Run not found")
      return { run: normalize(result.list[0]), evidence: await all("evidence", { run_id: id }) }
    }
    if (["settings", "crawler-sources", "citation-monitors", "providers"].includes(resource)) return { list: await all(resource, query) }
    const result: any = await read(resource, resource === "traffic-stats" ? { ...query, path_size: 10 } : query)
    if (resource === "site-audit") result.checks = result.checks.map((check: any) => ({ ...normalize(check), title: String(getI18n().t(`seo.checks.${check.key}`, { defaultValue: auditTitles[check.key] || check.key })) }))
    if (resource === "google-overview") {
      for (const provider of ["ga4", "gsc"]) if (result[provider]?.error) result[provider].error = localized(result[provider].error)
    }
    if (resource === "traffic-stats" && result.top_locations) result.top_locations = result.top_locations.map((item: any) => ({ ...item, name: /^[A-Z]{2}$/.test(item.name) ? new Intl.DisplayNames([getI18n().language === "zhCN" ? "zh-CN" : "en"], { type: "region" }).of(item.name) : item.name || (getI18n().language === "zhCN" ? "未知" : "Unknown") }))
    if (Array.isArray(result.list)) result.list = result.list.map(normalize)
    return result
  }
  if (method === "post" && resource === "crawler-sources" && id === "scan") return action("scan")
  if (method === "post" && resource === "google-connections") return action("google-test", undefined, undefined, { provider: id })
  if (method === "post" && resource === "indexnow") {
    const result = await action("indexnow", undefined, undefined, body)
    result.response_headers = Object.fromEntries(Object.entries(result.response_headers || {}).map(([name, value]) => [name, Array.isArray(value) ? value : [value]]))
    return result
  }
  if (method === "post" && resource === "citation-runs") return action("run", undefined, body.monitor_id)
  if (resource === "settings") {
    if (method === "delete") {
      const rows = await all("settings")
      const row = rows.find(row => row.key === id)
      if (row) return action("delete", resource, row.id)
      return { deleted: true }
    }
    return action("save", resource, undefined, { ...body, key: id })
  }
  if (method === "delete") return action("delete", resource, id)
  return action("save", resource, id, body)
}
async function request<T>(method: string, path: string, body?: any, params?: Record<string, any>) {
  try {
    return { data: { code: 0, message: "", data: await execute(method, path, body, params) } as T }
  } catch (error) {
    throw new Error(localized(error instanceof Error ? error.message : "seo.operationFailed"))
  }
}
export default {
  get: <T,>(path: string, options?: { params?: any }) => request<T>("get", path, undefined, options?.params),
  post: <T,>(path: string, body: any, _options?: { timeout?: number }) => request<T>("post", path, body),
  put: <T,>(path: string, body: any) => request<T>("put", path, body),
  delete: <T,>(path: string) => request<T>("delete", path),
}
