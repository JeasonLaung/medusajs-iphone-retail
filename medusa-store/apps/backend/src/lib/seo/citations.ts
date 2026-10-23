import type { MedusaContainer } from "@medusajs/framework/types"
import { decrypt, httpUrl, requestJSON, seoService } from "./core"

export function parseCitationResponse(body: any) {
  const evidence: { source_url: string; source_title: string; evidence_type: "provider_citation" | "answer_link" }[] = []
  function add(url: string, title: string, type: "provider_citation" | "answer_link") {
    try {
      const parsed = httpUrl(url)
      if (!evidence.some(e => e.source_url === parsed.href)) evidence.push({ source_url: parsed.href, source_title: title || "", evidence_type: type })
    } catch { /* Ignore malformed links; they are not citation evidence. */ }
  }
  const pieces: string[] = []
  if (body.choices?.[0]?.message) {
    const message = body.choices[0].message
    if (typeof message.content === "string") pieces.push(message.content)
    for (const item of message.annotations || []) if (item.url_citation?.url) add(item.url_citation.url, item.url_citation.title, "provider_citation")
  } else {
    for (const item of body.output || []) for (const content of item.content || []) {
      if (typeof content.text === "string") pieces.push(content.text)
      for (const a of content.annotations || []) if (a.type === "url_citation" && a.url) add(a.url, a.title, "provider_citation")
    }
    if (!pieces.length && typeof body.output_text === "string") pieces.push(body.output_text)
  }
  const answer = pieces.join("\n").trim()
  if (!answer || body.status === "incomplete" || body.error) throw new Error("seo.emptyAnswer")
  for (const raw of answer.match(/https?:\/\/[^\s<>"']+/g) || []) add(raw.replace(/[.,;:!?\])}]+$/, ""), "", "answer_link")
  return { answer, evidence }
}

export function providerRequest(provider: any, prompt: string) {
  const base = httpUrl(provider.base_url).href.replace(/\/$/, "")
  const suffix = provider.wire_api === "responses" ? "responses" : "chat/completions"
  const url = base.endsWith(`/${suffix}`) ? base : `${base}/${suffix}`
  const body: Record<string, unknown> = { model: provider.model }
  if (provider.wire_api === "responses") {
    body.input = prompt
    if (provider.provider === "chatgpt") body.tools = [{ type: "web_search" }]
  } else if (provider.wire_api === "chat_input") body.input = prompt
  else body.messages = [{ role: "user", content: prompt }]
  return { url, body }
}

export async function runCitation(container: MedusaContainer, monitorId: string) {
  const service = seoService(container)
  const locking = container.resolve("locking")
  return locking.execute(`seo-citation:${monitorId}`, async () => {
    const monitor = await service.retrieveSeoMonitor(monitorId)
    const started = new Date()
    // Recover interrupted runs, preserving an explicit failed state for operators.
    const stale = await service.listSeoRuns({ monitor_id: monitorId, status: "running" })
    for (const row of stale) await service.updateSeoRuns({ id: row.id, status: "failed", finished_at: started, error_message: "seo.interruptedRun" })
    const run = await service.createSeoRuns({ monitor_id: monitor.id, provider: monitor.provider, prompt_text: monitor.query_text, started_at: started })
    await service.updateSeoMonitors({ id: monitor.id, last_run_at: started })
    try {
      const [provider] = await service.listSeoProviders({ provider: monitor.provider })
      if (!provider?.api_key || !provider.model) throw new Error("seo.providerRequired")
      await service.updateSeoRuns({ id: run.id, model: provider.model })
      const { url, body } = providerRequest(provider, monitor.query_text)
      const response = await requestJSON(url, {
        method: "POST", headers: { Authorization: `Bearer ${decrypt(provider.api_key)}`, "Content-Type": "application/json" }, body: JSON.stringify(body),
      }, 60000)
      const { answer, evidence } = parseCitationResponse(response)
      if (evidence.length) await service.createSeoEvidences(evidence.map(item => ({ ...item, run_id: run.id })))
      return await service.updateSeoRuns({ id: run.id, model: provider.model, answer_text: answer, status: "completed",
        brand_mentioned: answer.toLowerCase().includes(monitor.brand_name.toLowerCase()), citation_count: evidence.length, finished_at: new Date(), error_message: "" })
    } catch (error) {
      // Do not store authorization headers, API keys, or provider response bodies.
      const message = error instanceof Error ? error.message : "seo.providerFailed"
      const safe = /^(seo\.|HTTP \d{3}$)/.test(message) ? message : "seo.providerFailed"
      return await service.updateSeoRuns({ id: run.id, status: "failed", finished_at: new Date(), error_message: safe })
    }
  }, { timeout: 5, expire: 180 })
}
