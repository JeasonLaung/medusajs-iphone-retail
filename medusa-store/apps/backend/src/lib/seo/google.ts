import type SeoModuleService from "../../modules/seo/service"
import { invalid, requestJSON, settings } from "./core"

async function accessToken(service: SeoModuleService, provider: string) {
  if (!["ga4", "gsc"].includes(provider)) return invalid("invalidProvider")
  const values = await settings(service, true), prefix = `${provider}.oauth.`
  if (!["client_id", "client_secret", "refresh_token"].every(k => values[prefix + k])) return invalid("googleCredentialsRequired")
  const token = await requestJSON("https://oauth2.googleapis.com/token", { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({ grant_type: "refresh_token", client_id: values[prefix + "client_id"], client_secret: values[prefix + "client_secret"], refresh_token: values[prefix + "refresh_token"] }).toString() })
  if (!token.access_token) throw new Error("seo.googleCredentialsRequired")
  return { token, values }
}
async function googleData(service: SeoModuleService, provider: "ga4" | "gsc", days: number) {
  const { token, values } = await accessToken(service, provider)
  const headers = { Authorization: `Bearer ${token.access_token}`, "Content-Type": "application/json" }
  if (provider === "ga4") {
    const property = values["ga4.property_id"]?.replace(/^properties\//, "")
    if (!property || !/^\d+$/.test(property)) return invalid("googlePropertyRequired")
    const data = await requestJSON(`https://analyticsdata.googleapis.com/v1beta/properties/${property}:runReport`, { method: "POST", headers,
      body: JSON.stringify({ dateRanges: [{ startDate: `${days}daysAgo`, endDate: "yesterday" }], dimensions: [{ name: "date" }], metrics: ["activeUsers", "newUsers", "sessions", "keyEvents"].map(name => ({ name })), metricAggregations: ["TOTAL"], limit: "100" }) })
    const metrics = (row: any) => Object.fromEntries(["active_users", "new_users", "sessions", "key_events"].map((k, i) => [k, Number(row?.metricValues?.[i]?.value || 0)]))
    return { connected: true, ...metrics(data.totals?.[0]), trend: (data.rows || []).map((r: any) => ({ date: String(r.dimensionValues?.[0]?.value || "").replace(/^(\d{4})(\d{2})(\d{2})$/, "$1-$2-$3"), ...metrics(r) })).sort((a: any, b: any) => a.date.localeCompare(b.date)) }
  }
  const resource = values["gsc.resource"]
  if (!resource) return invalid("googleResourceRequired")
  const end = new Date(Date.now() - 3 * 86400000), start = new Date(end.valueOf() - (days - 1) * 86400000)
  const data = await requestJSON(`https://www.googleapis.com/webmasters/v3/sites/${encodeURIComponent(resource)}/searchAnalytics/query`, { method: "POST", headers,
    body: JSON.stringify({ startDate: start.toISOString().slice(0, 10), endDate: end.toISOString().slice(0, 10), dimensions: ["date"], rowLimit: 100 }) })
  const rows = data.rows || [], clicks = rows.reduce((s: number, r: any) => s + r.clicks, 0), impressions = rows.reduce((s: number, r: any) => s + r.impressions, 0)
  return { connected: true, clicks, impressions, ctr: impressions ? clicks / impressions : 0,
    position: impressions ? rows.reduce((s: number, r: any) => s + r.position * r.impressions, 0) / impressions : 0,
    trend: rows.map((r: any) => ({ date: r.keys?.[0], clicks: r.clicks, impressions: r.impressions, ctr: r.ctr, position: r.position })) }
}
export async function googleOverview(service: SeoModuleService, days = 31) {
  const [ga4, gsc] = await Promise.all((["ga4", "gsc"] as const).map(async provider => {
    try { return await googleData(service, provider, days) } catch (e) {
      return { connected: false, error: e instanceof Error && e.message.startsWith("seo.") ? e.message : "seo.googleConnectionFailed", trend: [] }
    }
  }))
  return { days, ga4, gsc }
}
export async function testGoogle(service: SeoModuleService, provider: string) {
  if (provider !== "ga4" && provider !== "gsc") return invalid("invalidProvider")
  // A report request verifies the configured property's permissions, beyond just refreshing OAuth.
  await googleData(service, provider, 1)
  return { provider, connected: true }
}
