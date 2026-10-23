import { open, stat } from "node:fs/promises"
import path from "node:path"
import { isIP } from "node:net"
import type { MedusaContainer } from "@medusajs/framework/types"
import { allRows, hash, httpUrl, invalid, requestJSON, seoService, settings, siteUrl } from "./core"

export const crawlerNames = ["OAI-SearchBot", "ChatGPT-User", "GPTBot", "Claude-User", "ClaudeBot", "anthropic-ai", "Perplexity-User", "PerplexityBot", "Google-Extended", "GoogleOther", "Bytespider", "CCBot", "cohere-ai", "Applebot-Extended", "Meta-ExternalAgent", "Amazonbot", "YouBot", "Diffbot", "Bingbot", "Googlebot"]
export const identifyCrawler = (agent: string) => crawlerNames.find(name => agent.toLowerCase().includes(name.toLowerCase())) || ""
const aliases: Record<string, string> = { "chatgpt.com": "chatgpt", openai: "chatgpt", "openai.com": "chatgpt", fb: "facebook", "facebook.com": "facebook", "instagram.com": "instagram", "youtube.com": "youtube", "youtu.be": "youtube", "tiktok.com": "tiktok", "linkedin.com": "linkedin", "google.com": "google", "bing.com": "bing", "perplexity.ai": "perplexity", "claude.ai": "claude", "gemini.google.com": "gemini", "copilot.microsoft.com": "copilot" }
function sourceName(value: string) {
  value = value.trim().toLowerCase().replace(/^https?:\/\//, "").replace(/^www\./, "").split("/")[0]
  return aliases[value] || Object.entries(aliases).find(([host]) => host.includes(".") && value.endsWith(`.${host}`))?.[1] || value.slice(0, 128)
}
function platformOf(source: string) {
  if (["chatgpt", "perplexity", "claude", "gemini", "copilot"].includes(source)) return "ai_referral"
  return ({ facebook: "meta", instagram: "meta", meta: "meta", youtube: "google", google: "google", tiktok: "tiktok", linkedin: "linkedin", bing: "microsoft", microsoft: "microsoft" } as Record<string, string>)[source] || "referral"
}
export function classifyTraffic(url: URL, referrer: string, internalHost: string) {
  const q = url.searchParams, utm = sourceName(q.get("utm_source") || "")
  let referrerHost = ""
  try { referrerHost = new URL(referrer).hostname.toLowerCase().replace(/^www\./, "") } catch {}
  const clickRules = [
    ["fbclid", "meta_ads", "meta", "paid_social", "facebook,instagram,meta"], ["ttclid", "tiktok_ads", "tiktok", "paid_social", "tiktok"],
    ["gclid", "google_ads", "google", "paid", "google,youtube"], ["wbraid", "google_ads", "google", "paid", "google,youtube"], ["gbraid", "google_ads", "google", "paid", "google,youtube"],
    ["msclkid", "microsoft_ads", "bing", "paid_search", "bing,microsoft"], ["li_fat_id", "linkedin_ads", "linkedin", "paid_social", "linkedin"],
  ]
  for (const [key, platform, source, medium, compatible] of clickRules) if (q.get(key)) {
    const useUtm = compatible.split(",").includes(utm)
    return { platform, source: useUtm ? utm : source, medium: q.get("utm_medium")?.slice(0, 64) || medium, click_id_type: key, matched_by: useUtm ? "click_id+utm" : "click_id", referrer_host: referrerHost }
  }
  const internal = internalHost.replace(/^www\./, "")
  const source = utm || (referrerHost && referrerHost !== internal && !referrerHost.endsWith(`.${internal}`) && referrerHost !== "localhost" ? sourceName(referrerHost) : "")
  if (!source) return null
  const platform = platformOf(source)
  const medium = q.get("utm_medium")?.slice(0, 64) || (platform === "ai_referral" ? "ai_referral" : ["meta", "tiktok", "linkedin"].includes(platform) ? "social" : "referral")
  return { source, platform, medium, click_id_type: "", matched_by: utm ? "utm" : "referer", referrer_host: referrerHost }
}
export function parseLogLine(line: string, sourceId: string, internalHost: string) {
  const parts = line.match(/^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) ([^ ]+) [^"]*" (\d{3}) \S+ "([^"]*)" "([^"]*)"/)
  if (!parts) return null
  const [, ip, time, method, target, status, referrer, agent] = parts
  const occurred = new Date(time.replace(/^(\d{2})\/(\w{3})\/(\d{4}):/, "$2 $1 $3 "))
  if (Number.isNaN(occurred.valueOf())) return null
  let url: URL
  try { url = new URL(target, "http://nginx.local") } catch { return null }
  const crawler = identifyCrawler(agent), event_hash = hash(`${sourceId}\n${line}`), status_code = Number(status)
  if (crawler) return { kind: "crawler", value: { source_id: sourceId, crawler_name: crawler, request_path: `${url.pathname}${url.search}`, status_code, client_ip_hash: hash(ip), occurred_at: occurred, event_hash } }
  if (!/^(GET|HEAD)$/.test(method) || status_code < 200 || status_code >= 400 || /bot|spider|crawler|headless|monitor/i.test(agent)) return null
  if (/^\/(api|admin|app|_next|_astro|assets|static)(\/|$)/i.test(url.pathname) || /\.[a-z0-9]{1,8}$/i.test(url.pathname)) return null
  const attribution = classifyTraffic(url, referrer, internalHost)
  if (!attribution) return null
  return { kind: "traffic", value: { ...attribution, log_source_id: sourceId, landing_path: url.pathname, campaign: url.searchParams.get("utm_campaign")?.slice(0, 255) || "", status_code, client_ip: isIP(ip) ? ip : "", occurred_at: occurred, event_hash } }
}
export function allowedLogPath(value: string) {
  if (!path.isAbsolute(value)) return invalid("absoluteLogPathRequired")
  // Optional deployment allowlist; an empty list permits admin-selected local log files.
  const roots = (process.env.SEO_LOG_ROOTS || "").split(path.delimiter).filter(Boolean)
  const resolved = path.resolve(value)
  if (roots.length && !roots.some(root => { const rel = path.relative(path.resolve(root), resolved); return !rel.startsWith("..") && !path.isAbsolute(rel) })) return invalid("logPathDenied")
  return resolved
}
async function insertOnce(service: any, method: string, row: any) {
  const [existing] = await service[`list${method}`]({ event_hash: row.event_hash }, { take: 1 })
  if (existing) return false
  await service[`create${method}`](row)
  return true
}
export async function scanSources(container: MedusaContainer, force = true) {
  const service = seoService(container), values = await settings(service), host = new URL(siteUrl(values)).hostname
  return container.resolve("locking").execute("seo-log-scan", async () => {
    const sources = await allRows(service, "crawler-sources", { enabled: true }), summary = { sources: sources.length, succeeded: 0, failed: 0 }
    for (const source of sources) {
      if (!force && source.last_scanned_at && Date.now() - new Date(source.last_scanned_at).valueOf() < source.interval_minutes * 60000) continue
      try {
        const filePath = allowedLogPath(source.file_path), info = await stat(filePath)
        if (!info.isFile()) throw new Error("seo.logFileRequired")
        const file = await open(filePath, "r")
        try {
          // A stable first-line fingerprint detects replacement; byte offsets handle truncation.
          const head = Buffer.alloc(Math.min(65536, info.size)); await file.read(head, 0, head.length, 0)
          const newline = head.indexOf(10), fingerprint = newline < 0 ? "" : hash(head.subarray(0, newline + 1))
          const reset = Number(source.read_offset) > info.size || (!!source.file_fingerprint && fingerprint !== source.file_fingerprint)
          let offset = reset ? 0 : Number(source.read_offset), readLines = reset ? 0 : source.read_lines
          // Bound each poll to 4 MB; only advance past complete lines. Remaining bytes are read next time.
          const data = Buffer.alloc(Math.min(4 * 1024 * 1024, info.size - offset)); await file.read(data, 0, data.length, offset)
          const end = data.lastIndexOf(10)
          if (end >= 0) {
            for (const line of data.subarray(0, end + 1).toString("utf8").split("\n").slice(0, -1)) {
              readLines++
              const parsed = parseLogLine(line.replace(/\r$/, ""), source.id, host)
              if (parsed?.kind === "crawler" && source.collect_ai_crawlers) await insertOnce(service, "SeoCrawlerVisits", parsed.value)
              if (parsed?.kind === "traffic" && source.collect_traffic) await insertOnce(service, "SeoTrafficVisits", parsed.value)
            }
            offset += end + 1
          } else if (data.length === 4 * 1024 * 1024) throw new Error("seo.logLineTooLarge")
          const [, crawlerTotal] = await service.listAndCountSeoCrawlerVisits({ source_id: source.id }, { take: 1 })
          const [, trafficTotal] = await service.listAndCountSeoTrafficVisits({ log_source_id: source.id }, { take: 1 })
          await service.updateSeoSources({ id: source.id, read_offset: offset, read_lines: readLines, file_fingerprint: fingerprint,
            matched_lines: crawlerTotal, traffic_matched_lines: trafficTotal, last_scanned_at: new Date(), last_error: "" })
        } finally { await file.close() }
        summary.succeeded++
      } catch { summary.failed++; await service.updateSeoSources({ id: source.id, last_scanned_at: new Date(), last_error: "seo.logReadFailed" }) }
    }
    await enrichLocations(service)
    return summary
  }, { timeout: 5, expire: 600 })
}
async function enrichLocations(service: any) {
  const pending = await service.listSeoTrafficVisits({ location: "" }, { take: 25 })
  for (const ip of new Set<string>(pending.map((r: any) => r.client_ip))) {
    if (!isIP(ip) || /^(127\.|10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|::1$|fc|fd|fe80)/i.test(ip)) continue
    try {
      const geo = await requestJSON(`https://api.country.is/${encodeURIComponent(ip)}`, {}, 3000)
      if (/^[A-Z]{2}$/.test(geo.country)) {
        const records = await service.listSeoTrafficVisits({ client_ip: ip, location: "" }, { take: 1000 })
        if (records.length) await service.updateSeoTrafficVisits(records.map((r: any) => ({ id: r.id, location: geo.country })))
      }
    } catch { /* Failed lookups remain pending for a later cycle. */ }
  }
}
