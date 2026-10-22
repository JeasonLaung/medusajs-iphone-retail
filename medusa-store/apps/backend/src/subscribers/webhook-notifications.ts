import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import type { SubscriberArgs, SubscriberConfig } from "@medusajs/framework"
import { WEBHOOK_NOTIFICATIONS_MODULE } from "../modules/webhook-notifications"
import type WebhookNotificationsService from "../modules/webhook-notifications/service"
import { decrypt } from "../modules/webhook-notifications/security"
import { deliver, events } from "../modules/webhook-notifications/transport"
import type { WebhookEvent } from "../lib/webhook-events"
import { formatWebhookMessage, loadWebhookContext, webhookMessageText, webhookResourceId, type WebhookContext, type WebhookEventData } from "../modules/webhook-notifications/message"

export default async function webhookNotifications({ event, container }: SubscriberArgs<WebhookEventData>) {
  if (!events.includes(event.name as WebhookEvent)) return
  const name = event.name as WebhookEvent
  const resourceId = webhookResourceId(name, event.data)
  if (!resourceId) return
  const service: WebhookNotificationsService = container.resolve(WEBHOOK_NOTIFICATIONS_MODULE)
  const endpoints = await service.listWebhookEndpoints({ enabled: true })
  const logger = container.resolve(ContainerRegistrationKeys.LOGGER)
  let context: WebhookContext | undefined
  for (const endpoint of endpoints) {
    const selected = (endpoint.events as { values?: string[] })?.values || []
    if (!endpoint.enabled || !selected.includes(name)) continue
    try {
      context ||= await loadWebhookContext(container, name, event.data)
      const message = formatWebhookMessage(context, name, endpoint.language === "zh" ? "zh" : "en", endpoint.keyword)
      await deliver({ ...endpoint, url: decrypt(endpoint.url), secret: decrypt(endpoint.secret), token: decrypt(endpoint.token) }, webhookMessageText(message), name, resourceId, message)
      await service.updateWebhookEndpoints({ id: endpoint.id, last_status: "success", last_sent_at: new Date() })
    } catch {
      logger.warn(`Webhook notification failed: ${endpoint.id} (${event.name})`)
      await service.updateWebhookEndpoints({ id: endpoint.id, last_status: "failed", last_sent_at: new Date() }).catch(() => logger.warn(`Unable to update webhook status: ${endpoint.id}`))
    }
  }
}
export const config: SubscriberConfig = { event: [...events] }
