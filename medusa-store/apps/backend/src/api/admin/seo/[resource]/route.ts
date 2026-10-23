import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { allRows, invalid, resources, safeRecord, seoService, type Resource } from "../../../../lib/seo/core"
import { visitFilters, visitStats } from "../../../../lib/seo/analytics"
import { sitemapPages, siteAudit } from "../../../../lib/seo/publication"
import { googleOverview } from "../../../../lib/seo/google"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const service = seoService(req.scope), resource = req.params.resource, q = req.query as Record<string, any>
  if (resource === "overview") {
    const count = async (method: string, filters = {}) => (await (service as any)[method](filters, { take: 1 }))[1]
    const [monitors, enabled_monitors, completed_runs, mentioned_runs, failed_runs, provider_citations, answer_links] = await Promise.all([
      count("listAndCountSeoMonitors"), count("listAndCountSeoMonitors", { enabled: true }), count("listAndCountSeoRuns", { status: "completed" }),
      count("listAndCountSeoRuns", { status: "completed", brand_mentioned: true }), count("listAndCountSeoRuns", { status: "failed" }),
      count("listAndCountSeoEvidences", { evidence_type: "provider_citation" }), count("listAndCountSeoEvidences", { evidence_type: "answer_link" }),
    ])
    return res.json({ monitors, enabled_monitors, completed_runs, mentioned_runs, failed_runs, provider_citations, answer_links })
  }
  if (resource === "site-audit") return res.json(await siteAudit(service))
  if (resource === "sitemap-pages") {
    const [pages, submissions] = await Promise.all([sitemapPages(service), allRows(service, "submissions", { success: true })])
    const submitted = new Map<string, string>()
    for (const row of submissions) for (const url of row.urls?.list || []) {
      const date = new Date(row.submitted_at).toISOString()
      if (!submitted.has(url) || date > submitted.get(url)!) submitted.set(url, date)
    }
    return res.json({ list: pages.map(page => ({ ...page, last_submitted_at: submitted.get(page.url) || "" })) })
  }
  if (resource === "google-overview") return res.json(await googleOverview(service, Math.min(90, Math.max(1, Number(q.days) || 31))))
  if (resource === "crawler-stats" || resource === "traffic-stats") return res.json(await visitStats(service, q, resource === "traffic-stats"))
  if (!(resource in resources)) return invalid("invalidResource")
  const filters: Record<string, any> = resource === "traffic-visits" || resource === "crawler-visits" ? visitFilters(q, resource === "traffic-visits") : {}
  for (const key of resource === "settings" ? ["group"] : resource === "citation-runs" ? ["monitor_id", "status"] : resource === "evidence" ? ["run_id"] : []) if (q[key]) filters[key] = String(q[key])
  if (q.id) filters.id = String(q.id)
  if (q.q && resource === "page-metas") filters.path = { $like: `%${String(q.q).slice(0, 255)}%` }
  const page = Math.max(1, Math.floor(Number(q.page) || 1)), size = Math.min(200, Math.max(1, Math.floor(Number(q.size) || 20)))
  const suffix = resources[resource as Resource]
  const [list, count] = await (service as any)[`listAndCount${suffix}`](filters, { skip: (page - 1) * size, take: size, order: { created_at: "DESC", id: "DESC" } })
  res.json({ list: list.map((row: any) => safeRecord(resource, row)), total: count, page, size })
}
