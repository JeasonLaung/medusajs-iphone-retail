import { MedusaError } from "@medusajs/framework/utils"
import { createHmac } from "node:crypto"
import { lookup } from "node:dns/promises"
import { BlockList, isIP } from "node:net"
import { request } from "node:https"
import { webhookEvents } from "../../lib/webhook-events"
import type { WebhookMessage } from "./message"

export const providers = ["feishu", "wecom", "dingtalk", "slack", "mattermost", "generic"] as const
export const events = webhookEvents
export type Provider = typeof providers[number]
export type Target = { provider: Provider; url: string; secret: string; token: string }
const blocked = new BlockList()
for (const [ip, prefix] of [["0.0.0.0", 8], ["10.0.0.0", 8], ["100.64.0.0", 10], ["127.0.0.0", 8], ["169.254.0.0", 16], ["172.16.0.0", 12], ["192.0.0.0", 24], ["192.168.0.0", 16], ["198.18.0.0", 15], ["224.0.0.0", 3]] as const) blocked.addSubnet(ip, prefix)
export function publicAddress(address: string) {
  return isIP(address) === 4 ? !blocked.check(address) : isIP(address) === 6 && /^[23][0-9a-f]{3}:/i.test(address)
}
export function validateUrl(value: string, provider: Provider) {
  let url: URL
  try { url = new URL(value) } catch { throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.url") }
  if (url.protocol !== "https:" || url.username || url.password || url.hash || (url.port && url.port !== "443")) throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.url")
  const hosts: Partial<Record<Provider, string[]>> = { feishu: ["open.feishu.cn", "open.larksuite.com"], wecom: ["qyapi.weixin.qq.com"], dingtalk: ["oapi.dingtalk.com"], slack: ["hooks.slack.com"] }
  if (hosts[provider] && !hosts[provider]!.includes(url.hostname)) throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.url")
  const host = url.hostname.replace(/^\[|\]$/g, "")
  if (host === "localhost" || host.endsWith(".localhost") || (isIP(host) && !publicAddress(host))) throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.url")
  return url
}
export function buildRequest(target: Target, text: string, event: string, id: string, now = Date.now(), message?: WebhookMessage) {
  const url = validateUrl(target.url, target.provider)
  let body: Record<string, unknown>
  if (target.provider === "feishu") {
    if (message) {
      const paragraphs: Record<string, string>[][] = message.lines.map(line => [{ tag: "text", text: line }])
      const links = message.links.map(link => [{ tag: "a", text: link.label, href: link.url }])
      const post = { title: message.title, content: [...paragraphs, ...links] }
      body = { msg_type: "post", content: { post: { [message.language === "zh" ? "zh_cn" : "en_us"]: post } } }
      // Feishu limits robot messages to 20 KB. Keep the action links when shortening long orders.
      let shortened = false
      while (Buffer.byteLength(JSON.stringify(body)) > 18500 && paragraphs.length > 1) {
        paragraphs.pop()
        shortened = true
        post.content = [...paragraphs, ...links]
      }
      if (shortened) post.content = [...paragraphs, [{ tag: "text", text: message.language === "zh" ? "更多明细请点击链接查看。" : "Follow the link for more details." }], ...links]
    } else body = { msg_type: "text", content: { text } }
    if (target.secret) {
      const timestamp = String(Math.floor(now / 1000))
      body.timestamp = timestamp
      body.sign = createHmac("sha256", `${timestamp}\n${target.secret}`).update("").digest("base64")
    }
  } else if (target.provider === "wecom" || target.provider === "dingtalk") {
    if (target.provider === "wecom" && message && Buffer.byteLength(text) > 2000) {
      const lines = [...message.lines]
      const suffix = message.links.map(link => `${link.label}: ${link.url}`).join("\n")
      do { lines.pop(); text = [message.title, ...lines, "…", suffix].join("\n") } while (lines.length && Buffer.byteLength(text) > 2000)
    }
    body = { msgtype: "text", text: { content: text } }
    if (target.provider === "dingtalk" && target.secret) {
      url.searchParams.set("timestamp", String(now))
      url.searchParams.set("sign", createHmac("sha256", target.secret).update(`${now}\n${target.secret}`).digest("base64"))
    }
  } else if (target.provider === "generic") {
    body = { event, resource_id: id, text, timestamp: new Date(now).toISOString(), ...(message ? { notification: message } : {}) }
  } else body = { text: target.provider === "slack" && message ? text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;") : text }
  return { url, body }
}
export async function deliver(target: Target, text: string, event: string, id: string, message?: WebhookMessage) {
  const { url, body } = buildRequest(target, text, event, id, Date.now(), message)
  const addresses = await lookup(url.hostname, { all: true })
  if (!addresses.length || addresses.some(a => !publicAddress(a.address))) throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.url")
  const selected = addresses[0]
  const payload = JSON.stringify(body)
  // Pin the checked DNS result to this connection and refuse redirects.
  const result = await new Promise<string>((resolve, reject) => {
    const req = request(url, { method: "POST", lookup: (_host, options, cb) => {
      // Node's automatic family selection requests an array when all is true.
      if (options.all) cb(null, [selected])
      else cb(null, selected.address, selected.family)
    }, headers: { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(payload), ...(target.provider === "generic" && target.token ? { Authorization: `Bearer ${target.token}` } : {}) } }, res => {
      let response = ""
      res.on("data", chunk => { response += chunk; if (response.length > 65536) req.destroy(new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.delivery")) })
      res.on("error", reject)
      res.on("end", () => res.statusCode && res.statusCode >= 200 && res.statusCode < 300 ? resolve(response) : reject(new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.delivery")))
    })
    const timer = setTimeout(() => req.destroy(new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.delivery")), 10000)
    req.on("close", () => clearTimeout(timer))
    req.on("error", () => reject(new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.delivery")))
    req.end(payload)
  })
  if (["feishu", "wecom", "dingtalk"].includes(target.provider)) {
    let response: { code?: number; errcode?: number; StatusCode?: number }
    try { response = JSON.parse(result) } catch { throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.delivery") }
    if ((response.code ?? response.errcode ?? response.StatusCode) !== 0) throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.delivery")
  }
}
