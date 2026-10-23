import assert from "node:assert/strict"
import { createServer } from "node:http"
import { writeFile, appendFile, unlink } from "node:fs/promises"
import path from "node:path"
import type { ExecArgs } from "@medusajs/framework/types"
import { seoCommandWorkflow } from "../workflows/seo-command"
import { decrypt, seoService } from "../lib/seo/core"
import { publicConfig, sitemapPages, siteAudit } from "../lib/seo/publication"

/** Ephemeral SEO-only integration fixtures; no commerce data or existing configuration is changed. */
export default async function verifySEO({ container }: ExecArgs) {
  const service = seoService(container), tag = `seo-check-${Date.now()}`
  const created: [string, string][] = [], checks: string[] = []
  const file = path.resolve("../../../logs", `${tag}.access.log`)
  const command = async (input: any) => (await seoCommandWorkflow(container).run({ input })).result
  const save = async (resource: string, data: any) => {
    const row = await command({ action: "save", resource, data })
    created.push([resource, row.id]); return row
  }
  const server = createServer(async (req, res) => {
    let body = ""; for await (const chunk of req) body += chunk
    assert.equal(req.url, "/v1/responses")
    assert.equal(JSON.parse(body).tools[0].type, "web_search")
    assert.equal(req.headers.authorization, "Bearer fixture-only-key")
    res.setHeader("content-type", "application/json")
    res.end(JSON.stringify({ output: [{ content: [{ text: "EuroRefurb reference https://example.com/product", annotations: [{ type: "url_citation", url: "https://example.com/product", title: "Fixture evidence" }] }] }] }))
  })
  try {
    const secret = await save("settings", { key: `test.${tag}.api_key`, value: "fixture-secret" })
    assert.equal(secret.value, "********")
    assert.equal(decrypt((await service.retrieveSeoSetting(secret.id)).value), "fixture-secret")
    assert.equal((await publicConfig(service)).settings[`test.${tag}.api_key`], undefined)
    checks.push("Encrypted secret persists; public config excludes it")

    const publication = { "seo.indexnow_enabled": "true", "seo.indexnow_key": tag, "bing.verification_enabled": "true", "bing.site_auth_xml": "<users><user>fixture-verification</user></users>", "seo.robots_txt": `User-agent: *\n# ${tag}`, "seo.llms_txt": `# ${tag}` }
    const existingPublication = await service.listSeoSettings({ key: { $in: Object.keys(publication) } })
    if (!existingPublication.length) {
      for (const [key, value] of Object.entries(publication)) await save("settings", { key, value })
      for (const [route, expected] of [[`/${tag}.txt`, tag], ["/BingSiteAuth.xml", "fixture-verification"], ["/robots.txt", tag], ["/llms.txt", tag]]) {
        const response = await fetch(`http://localhost:8000${route}`, { headers: { "cache-control": "no-cache" } })
        assert.equal(response.status, 200, route); assert((await response.text()).includes(expected), route)
      }
      checks.push("Database settings publish live IndexNow key / Bing XML / robots / llms resources")
      // Restore normal discovery resources before auditing the actual site.
      for (let i = created.length - 1; i >= 0; i--) if (created[i][0] === "settings") {
        await command({ action: "delete", resource: created[i][0], id: created[i][1] }); created.splice(i, 1)
      }
    }
    // Local integration checks must not depend on a development tunnel. Keep a
    // merchant's explicit origin intact; a fixture override is removed in finally.
    if (!(await service.listSeoSettings({ key: "seo.site_url" })).length) {
      await save("settings", { key: "seo.site_url", value: "http://localhost:8000" })
    }
    const map = await sitemapPages(service)
    assert(map.length > 0); assert(map.every(page => !/\/(account|checkout|cart|order)\//.test(page.path)))
    checks.push(`Full live sitemap index traversal: ${map.length} canonical public URLs`)
    const audit = await siteAudit(service)
    assert(audit.checks.every(check => check.passed), JSON.stringify(audit.checks.filter(check => !check.passed)))
    checks.push(`Live site audit: ${audit.checks.length} checks passed`)

    const [existingPage] = await service.listSeoPages({ path: "/dk-en/about" })
    if (!existingPage) {
      await save("page-metas", { path: "/dk-en/about", title: tag, description: "Ephemeral SEO integration verification", structured_data: { "@context": "https://schema.org", "@type": "WebPage", name: tag } })
      const response = await fetch("http://localhost:8000/dk-en/about", { headers: { "cache-control": "no-cache" } })
      const html = await response.text()
      assert(html.includes(`<title>${tag}</title>`), `Database override reaches real storefront title: HTTP ${response.status}, ${html.match(/<title>[^<]*<\/title>/)?.[0]}`)
      assert(html.includes(`"name":"${tag}"`), "Database JSON-LD reaches real storefront")
      assert(html.includes('rel="canonical"'), "Canonical preserved")
      checks.push("Workflow → database → public API → real SSR title / JSON-LD / canonical")
    }

    const existingSources = await service.listSeoSources({ enabled: true })
    if (!existingSources.length) {
      const line = (second: string, target: string, ref: string, agent: string) => `127.0.0.1 - - [23/Oct/2026:12:00:${second} +0800] "GET ${target} HTTP/1.1" 200 123 "${ref}" "${agent}"`
      await writeFile(file, line("01", "/dk-en/about", "-", "GPTBot") + "\n" + line("02", "/dk-en/about?utm_source=chatgpt.com", "-", "Mozilla/5.0") + "\n")
      const source = await save("crawler-sources", { source_name: tag, file_path: file, interval_minutes: 10080 })
      await service.updateSeoSources({ id: source.id, last_scanned_at: new Date() })
      await command({ action: "scan" }); await command({ action: "scan" })
      assert.equal((await service.listAndCountSeoCrawlerVisits({ source_id: source.id }))[1], 1)
      assert.equal((await service.listAndCountSeoTrafficVisits({ log_source_id: source.id }))[1], 1)
      await appendFile(file, line("03", "/dk-en/help?gclid=fixture", "-", "Mozilla/5.0"))
      await command({ action: "scan" })
      assert.equal((await service.listAndCountSeoTrafficVisits({ log_source_id: source.id }))[1], 1)
      await appendFile(file, "\n"); await command({ action: "scan" })
      assert.equal((await service.listAndCountSeoTrafficVisits({ log_source_id: source.id }))[1], 2)
      await writeFile(file, line("04", "/dk-en/help", "-", "ClaudeBot") + "\n")
      await command({ action: "scan" })
      const updated = await service.retrieveSeoSource(source.id)
      assert.equal(updated.matched_lines, 2); assert.equal(updated.traffic_matched_lines, 2)
      checks.push("Real file scan: crawler / attribution / repeat dedup / partial line / rotation / counters")
    }

    const [provider] = await service.listSeoProviders({ provider: "chatgpt" })
    if (!provider) {
      await new Promise<void>(resolve => server.listen(0, "127.0.0.1", resolve))
      const address = server.address() as { port: number }
      await save("providers", { provider: "chatgpt", api_key: "fixture-only-key", base_url: `http://127.0.0.1:${address.port}/v1`, model: "fixture-model", wire_api: "responses" })
      const monitor = await save("citation-monitors", { monitor_name: tag, brand_name: "EuroRefurb", provider: "chatgpt", query_text: "Fixture query", enabled: false })
      const run = await command({ action: "run", id: monitor.id })
      assert.equal(run.status, "completed"); assert.equal(run.brand_mentioned, true); assert.equal(run.citation_count, 1)
      const [evidence] = await service.listSeoEvidences({ run_id: run.id })
      assert.equal(evidence.evidence_type, "provider_citation")
      assert.equal(evidence.source_url, "https://example.com/product")
      checks.push("Workflow → local provider fixture → completed run → persisted citation evidence")
    }
    console.log(JSON.stringify({ checks, passed: checks.length }))
  } finally {
    for (const [resource, id] of created.reverse()) {
      if (resource === "citation-monitors") {
        const runs = await service.listSeoRuns({ monitor_id: id })
        for (const run of runs) {
          const evidence = await service.listSeoEvidences({ run_id: run.id })
          if (evidence.length) await service.deleteSeoEvidences(evidence.map(row => row.id))
        }
        if (runs.length) await service.deleteSeoRuns(runs.map(row => row.id))
      }
      if (resource === "crawler-sources") {
        const crawlers = await service.listSeoCrawlerVisits({ source_id: id }), visits = await service.listSeoTrafficVisits({ log_source_id: id })
        if (crawlers.length) await service.deleteSeoCrawlerVisits(crawlers.map(row => row.id))
        if (visits.length) await service.deleteSeoTrafficVisits(visits.map(row => row.id))
      }
      await command({ action: "delete", resource, id })
    }
    if (server.listening) await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
    await unlink(file).catch(() => {})
    console.log("Ephemeral SEO fixtures cleaned up")
  }
}
