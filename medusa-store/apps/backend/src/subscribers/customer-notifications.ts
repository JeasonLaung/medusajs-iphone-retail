import type { SubscriberArgs, SubscriberConfig } from "@medusajs/framework"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { createCustomerNoticeWorkflow } from "../workflows/customer-notifications"
import { loadWebhookContext, type WebhookEventData } from "../modules/webhook-notifications/message"
import type { WebhookEvent } from "../lib/webhook-events"

export default async function customerNotifications({ event, container }: SubscriberArgs<WebhookEventData>) {
  const context = await loadWebhookContext(container, event.name as WebhookEvent, event.data)
  if (!context.order) return
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data: orders } = await query.graph({ entity: "order", fields: ["id", "customer_id"], filters: { id: context.order.id } })
  if (!orders[0]?.customer_id) return
  await createCustomerNoticeWorkflow(container).run({ input: {
    customer_id: orders[0].customer_id,
    event_key: `${event.name}:${context.resourceId}`,
    kind: event.name,
    resource_id: context.order.id,
    reference: String(context.order.custom_display_id || context.order.display_id || context.order.id),
  } })
}
export const config: SubscriberConfig = { event: ["shipment.created", "delivery.created", "order.canceled", "after_sales.processing", "after_sales.completed", "after_sales.rejected"] }
