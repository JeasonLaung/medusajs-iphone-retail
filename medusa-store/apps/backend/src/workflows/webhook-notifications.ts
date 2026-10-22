import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { MedusaError } from "@medusajs/framework/utils"
import { z } from "@medusajs/framework/zod"
import { WEBHOOK_NOTIFICATIONS_MODULE } from "../modules/webhook-notifications"
import type WebhookNotificationsService from "../modules/webhook-notifications/service"
import { encrypt, decrypt } from "../modules/webhook-notifications/security"
import { deliver, events, providers, validateUrl } from "../modules/webhook-notifications/transport"
import { webhookAdminLink, webhookMessageText, type WebhookMessage } from "../modules/webhook-notifications/message"

const schema = z.object({
  name: z.string().trim().min(1).max(100),
  provider: z.enum(providers),
  url: z.string().trim().max(4096).optional(),
  secret: z.string().trim().max(1024).optional(),
  token: z.string().trim().max(2048).refine(v => !/[\r\n]/.test(v)).optional(),
  keyword: z.string().trim().max(100).default(""),
  enabled: z.boolean(),
  language: z.enum(["zh", "en"]),
  events: z.array(z.enum(events)).min(1).max(events.length),
})
type Input = { action: "save" | "delete" | "test"; id?: string; data?: unknown }
export function serializeEndpoint(endpoint: any) {
  const { url, secret, token, ...safe } = endpoint
  return { ...safe, has_url: !!url, has_secret: !!secret, has_token: !!token }
}
const operateWebhookNotificationStep = createStep("operate-webhook-notification", async (input: Input, { container }) => {
  const service: WebhookNotificationsService = container.resolve(WEBHOOK_NOTIFICATIONS_MODULE)
  const saved = input.id ? await service.retrieveWebhookEndpoint(input.id) : null
  if (input.action === "delete") {
    if (!saved) throw new MedusaError(MedusaError.Types.NOT_FOUND, "webhooks.errors.missing")
    await service.deleteWebhookEndpoints(saved.id)
    return new StepResponse({ id: saved.id })
  }
  if (input.action === "test") {
    if (!saved) throw new MedusaError(MedusaError.Types.NOT_FOUND, "webhooks.errors.missing")
    try {
      const zh = saved.language === "zh"
      const message: WebhookMessage = {
        title: zh ? "Webhook 测试通知" : "Webhook test notification", language: zh ? "zh" : "en",
        lines: [saved.keyword, zh ? "连接成功。业务通知将包含订单号、邮箱、收货地址、商品 SKU、金额、时间和处理链接。" : "Connected. Business notifications include order number, email, shipping address, SKUs, amounts, timestamps and action links."].filter(Boolean),
        links: [{ label: zh ? "打开后台订单列表" : "Open Admin orders", url: webhookAdminLink() }],
      }
      await deliver({ ...saved, url: decrypt(saved.url), secret: decrypt(saved.secret), token: decrypt(saved.token) }, webhookMessageText(message), "test", saved.id, message)
      await service.updateWebhookEndpoints({ id: saved.id, last_status: "success", last_sent_at: new Date() })
    } catch {
      await service.updateWebhookEndpoints({ id: saved.id, last_status: "failed", last_sent_at: new Date() })
      throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.delivery")
    }
    return new StepResponse({ id: saved.id })
  }
  const parsed = schema.safeParse(input.data)
  if (!parsed.success) throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.validation")
  const { url, secret, token, ...data } = parsed.data
  if (saved && saved.provider !== data.provider && !url) throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.url")
  try { validateUrl(url || (saved ? decrypt(saved.url) : ""), data.provider) } catch {
    throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.url")
  }
  let credentials: { url?: string; secret?: string; token?: string }
  try {
    credentials = {
      ...(url ? { url: encrypt(url) } : {}),
      ...(secret !== undefined ? { secret: encrypt(secret) } : saved?.provider !== data.provider ? { secret: "" } : {}),
      ...(token !== undefined ? { token: encrypt(token) } : saved?.provider !== data.provider ? { token: "" } : {}),
    }
  } catch { throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.encryption") }
  const endpoint = saved
    ? await service.updateWebhookEndpoints({ id: saved.id, ...data, ...credentials, events: { values: data.events } })
    : await service.createWebhookEndpoints({ ...data, ...credentials, url: credentials.url!, events: { values: data.events } })
  return new StepResponse(serializeEndpoint(endpoint))
})
export const webhookNotificationsWorkflow = createWorkflow("webhook-notifications", (input: Input) => new WorkflowResponse(operateWebhookNotificationStep(input)))
