import { createCipheriv, createDecipheriv, createHash, randomBytes } from "node:crypto"
import { MedusaError } from "@medusajs/framework/utils"
import type { MedusaContainer } from "@medusajs/framework/types"
import type SeoModuleService from "../../modules/seo/service"

export const seoService = (container: MedusaContainer): SeoModuleService => container.resolve("seo")
export const invalid = (key: string): never => { throw new MedusaError(MedusaError.Types.INVALID_DATA, `seo.${key}`) }
export const hash = (value: string | Buffer) => createHash("sha256").update(value).digest("hex")
function encryptionKey() {
  const key = process.env.SEO_ENCRYPTION_KEY || process.env.COOKIE_SECRET
  if (!key || key === "supersecret") return invalid("encryptionRequired")
  return createHash("sha256").update(key).digest()
}
export function encrypt(value: string) {
  if (!value) return ""
  const iv = randomBytes(12), cipher = createCipheriv("aes-256-gcm", encryptionKey(), iv)
  const content = Buffer.concat([cipher.update(value, "utf8"), cipher.final()])
  return [iv, cipher.getAuthTag(), content].map(v => v.toString("base64")).join(".")
}
export function decrypt(value: string) {
  if (!value) return ""
  const [iv, tag, content] = value.split(".").map(v => Buffer.from(v, "base64"))
  const cipher = createDecipheriv("aes-256-gcm", encryptionKey(), iv)
  cipher.setAuthTag(tag)
  return Buffer.concat([cipher.update(content), cipher.final()]).toString("utf8")
}
export const secretSetting = (key: string) => /(?:client_secret|refresh_token|api_key|access_token)$/.test(key)
export function safeRecord(resource: string, row: any) {
  if (resource === "settings") return { ...row, value: row.is_secret ? (row.value ? "********" : "") : row.value }
  if (resource === "providers") { const { api_key, ...safe } = row; return { ...safe, api_key_configured: !!api_key } }
  return row
}
export async function settings(service: SeoModuleService, includeSecrets = false) {
  const rows = await service.listSeoSettings({}, { take: 1000 })
  return Object.fromEntries(rows.filter(r => includeSecrets || !r.is_secret).map(r => [r.key, r.is_secret ? decrypt(r.value) : r.value]))
}
export function httpUrl(value: string) {
  let url: URL
  try { url = new URL(value) } catch { return invalid("invalidUrl") }
  if (!["http:", "https:"].includes(url.protocol) || url.username || url.password) return invalid("invalidUrl")
  return url
}
export function siteUrl(values: Record<string, string>) {
  const value = values["seo.site_url"] || process.env.STOREFRONT_URL || "http://localhost:8000"
  return httpUrl(value).origin
}
export function normalizePath(value: string) {
  if (!value.startsWith("/") || value.startsWith("//") || /[?#\\\x00-\x1f]/.test(value)) return invalid("invalidPath")
  return value === "/" ? "/" : value.replace(/\/+$/, "")
}
export function jsonValue(value: unknown, fallback: any) {
  if (typeof value !== "string") return value ?? fallback
  try { return JSON.parse(value || JSON.stringify(fallback)) } catch { return invalid("invalidJson") }
}
export async function requestText(url: string, init: RequestInit = {}, timeout = 20000, limit = 2 * 1024 * 1024) {
  httpUrl(url)
  const response = await fetch(url, { ...init, signal: AbortSignal.timeout(timeout), redirect: "error" })
  if (Number(response.headers.get("content-length")) > limit) throw new Error("seo.responseTooLarge")
  const reader = response.body?.getReader(), chunks: Uint8Array[] = []; let length = 0
  if (reader) try {
    for (;;) { const { done, value } = await reader.read(); if (done) break; length += value.length; if (length > limit) throw new Error("seo.responseTooLarge"); chunks.push(value) }
  } finally { await reader?.cancel().catch(() => {}) }
  return { response, text: Buffer.concat(chunks).toString("utf8") }
}
export async function requestJSON(url: string, init: RequestInit = {}, timeout = 20000) {
  const { response, text } = await requestText(url, init, timeout, 10 * 1024 * 1024)
  if (!response.ok) throw new Error(`HTTP ${response.status}`)
  return JSON.parse(text)
}
export const resources = {
  settings: "SeoSettings", "page-metas": "SeoPages", "crawler-sources": "SeoSources",
  "crawler-visits": "SeoCrawlerVisits", "traffic-visits": "SeoTrafficVisits", providers: "SeoProviders",
  "citation-monitors": "SeoMonitors", "citation-runs": "SeoRuns", evidence: "SeoEvidences", submissions: "SeoSubmissions",
} as const
export type Resource = keyof typeof resources
export async function allRows(service: SeoModuleService, resource: Resource, filters: Record<string, any> = {}) {
  const rows: any[] = []; let skip = 0
  for (;;) { const page = await (service as any)[`list${resources[resource]}`](filters, { skip, take: 1000, order: { created_at: "ASC", id: "ASC" } }); rows.push(...page); if (page.length < 1000) return rows; skip += page.length }
}
