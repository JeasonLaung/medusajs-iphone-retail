import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../../../../../modules/devices"
import type DevicesModuleService from "../../../../../modules/devices/service"
import { buildOrderTimeline } from "../../../../../lib/order-timeline"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const { data } = await query.graph({
    entity: "order",
    fields: [
      "id", "created_at", "canceled_at", "currency_code",
      "payment_collections.payments.id", "payment_collections.payments.amount", "payment_collections.payments.captured_at",
      "payment_collections.payments.captures.id", "payment_collections.payments.captures.amount", "payment_collections.payments.captures.created_at",
      "payment_collections.payments.refunds.id", "payment_collections.payments.refunds.amount", "payment_collections.payments.refunds.created_at",
      "fulfillments.labels.tracking_number", "fulfillments.labels.tracking_url",
      "fulfillments.id", "fulfillments.created_at", "fulfillments.shipped_at", "fulfillments.delivered_at", "fulfillments.canceled_at",
    ],
    filters: { id: req.params.id, customer_id: req.auth_context.actor_id },
  })
  const order = data[0]
  if (!order) throw new MedusaError(MedusaError.Types.NOT_FOUND, "Order not found")
  const devices: DevicesModuleService = req.scope.resolve(DEVICES_MODULE)
  const events = await devices.listDeviceEvents(
    { order_id: order.id, action: ["assign", "release", "replace_allocation"] },
    { select: ["id", "action", "created_at"], take: null, order: { created_at: "ASC" } },
  )
  res.json({
    events: buildOrderTimeline(order, events),
    currency_code: order.currency_code,
    shipments: (order.fulfillments || [])
      .filter((fulfillment): fulfillment is NonNullable<typeof fulfillment> => Boolean(fulfillment && !fulfillment.canceled_at && fulfillment.shipped_at))
      .map((fulfillment) => ({
        id: fulfillment.id,
        shipped_at: fulfillment.shipped_at,
        delivered_at: fulfillment.delivered_at,
        labels: (fulfillment.labels || []).map((label) => ({
          tracking_number: label.tracking_number,
          tracking_url: label.tracking_url,
        })),
      })),
  })
}
