import type SeoModuleService from "../../modules/seo/service"
import { allRows, invalid } from "./core"

export function visitFilters(q: Record<string, any>, traffic: boolean) {
  const result: Record<string, any> = {}
  for (const key of traffic ? ["source", "platform", "log_source_id"] : ["source_id", "crawler_name"]) if (q[key]) result[key] = String(q[key])
  if (q.crawler) result.crawler_name = String(q.crawler)
  if (q.path) result[traffic ? "landing_path" : "request_path"] = { $like: `%${String(q.path).slice(0, 512)}%` }
  if (traffic && q.locale) {
    if (!/^[a-z]{2}$/.test(q.locale)) return invalid("invalidLocale")
    const language = { $or: [{ landing_path: { $like: `/__-${q.locale}/%` } }, { landing_path: { $like: `/__-${q.locale}` } }] }
    if (result.landing_path) {
      const path = result.landing_path
      delete result.landing_path
      result.$and = [{ landing_path: path }, language]
    } else Object.assign(result, language)
  }
  const date = (v: unknown) => { const d = new Date(/^\d+$/.test(String(v)) ? Number(v) * 1000 : String(v)); if (Number.isNaN(d.valueOf())) return invalid("invalidDate"); return d }
  if (q.start || q.end) {
    result.occurred_at = { ...(q.start ? { $gte: date(q.start) } : {}), ...(q.end ? { $lte: date(q.end) } : {}) }
    if (q.start && q.end && date(q.start) > date(q.end)) return invalid("invalidDate")
  }
  return result
}
function ranked(rows: any[], key: string) {
  const counts = new Map<string, number>()
  rows.forEach(row => counts.set(String(row[key] || ""), (counts.get(String(row[key] || "")) || 0) + 1))
  return [...counts].map(([name, count]) => ({ name, count })).sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
}
export async function visitStats(service: SeoModuleService, q: Record<string, any>, traffic: boolean) {
  const rows = await allRows(service, traffic ? "traffic-visits" : "crawler-visits", visitFilters(q, traffic))
  const dated = rows.map(row => ({ date: new Date(row.occurred_at).toISOString().slice(0, 10) }))
  const trend = ranked(dated, "date").map(r => ({ date: r.name, count: r.count })).sort((a, b) => a.date.localeCompare(b.date))
  if (!traffic) return { total_count: rows.length, top_crawlers: ranked(rows, "crawler_name"), top_paths: ranked(rows, "request_path").slice(0, 100), trend }
  const pathQuery = String(q.path_query || "").toLowerCase(), pathSource = String(q.path_source || "")
  const grouped = new Map<string, { rows: any[]; unique: Set<string> }>()
  for (const row of rows) {
    if (pathQuery && !row.landing_path.toLowerCase().includes(pathQuery)) continue
    if (pathSource && row.source !== pathSource) continue
    const entry = grouped.get(row.landing_path) || { rows: [], unique: new Set<string>() }
    entry.rows.push(row); entry.unique.add(`${new Date(row.occurred_at).toISOString().slice(0, 10)}:${row.client_ip}`); grouped.set(row.landing_path, entry)
  }
  const paths = [...grouped].map(([name, value]) => {
    const sources = ranked(value.rows, "source")
    return { name, count: value.rows.length, daily_unique_ip_count: value.unique.size,
      sources: sources.length <= 5 ? sources : [...sources.slice(0, 5), { name: "other", count: sources.slice(5).reduce((s, r) => s + r.count, 0) }] }
  }).sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
  const page = Math.max(1, Number(q.path_page) || 1), size = Math.min(200, Math.max(1, Number(q.path_size) || 20))
  return { total_count: rows.length, top_sources: ranked(rows, "source"), top_platforms: ranked(rows, "platform"), top_locations: ranked(rows, "location"),
    top_paths: paths.slice((page - 1) * size, page * size), top_paths_total: paths.length, trend }
}
