import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import type { SubscriberArgs, SubscriberConfig } from "@medusajs/framework"
import { WEBHOOK_NOTIFICATIONS_MODULE } from "../modules/webhook-notifications"
import type WebhookNotificationsService from "../modules/webhook-notifications/service"
import { decrypt } from "../modules/webhook-notifications/security"
import { deliver, events } from "../modules/webhook-notifications/transport"

const labels: Record<string, [string, string]> = {
  "order.placed": ["新订单", "Order placed"],
  "order.canceled": ["订单取消", "Order canceled"],
  "payment.captured": ["付款已收取", "Payment captured"],
  "shipment.created": ["已创建发货", "Shipment created"],
}
export default async function webhookNotifications({ event, container }: SubscriberArgs<{ id: string }>) {
  const service: WebhookNotificationsService = container.resolve(WEBHOOK_NOTIFICATIONS_MODULE)
  const endpoints = await service.listWebhookEndpoints({ enabled: true })
  const logger = container.resolve(ContainerRegistrationKeys.LOGGER)
  for (const endpoint of endpoints) {
    const selected = (endpoint.events as { values?: string[] })?.values || []
    if (!selected.includes(event.name)) continue
    try {
      const label = labels[event.name][endpoint.language === "zh" ? 0 : 1]
      // Send only the event and resource identifier, without customer details.
      await deliver({ ...endpoint, url: decrypt(endpoint.url), secret: decrypt(endpoint.secret), token: decrypt(endpoint.token) }, `${endpoint.keyword ? endpoint.keyword + "\n" : ""}${label}\nID: ${event.data.id}`, event.name, event.data.id)
      await service.updateWebhookEndpoints({ id: endpoint.id, last_status: "success", last_sent_at: new Date() })
    } catch {
      logger.warn(`Webhook notification failed: ${endpoint.id} (${event.name})`)
      await service.updateWebhookEndpoints({ id: endpoint.id, last_status: "failed", last_sent_at: new Date() }).catch(() => logger.warn(`Unable to update webhook status: ${endpoint.id}`))
    }
  }
}
export const config: SubscriberConfig = { event: [...events] }
