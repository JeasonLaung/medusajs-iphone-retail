import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"

// This guest preview exposes only native shipping data of explicitly marked
// development fixtures, never customer identities or non-demo orders.
export async function GET(req: MedusaRequest, res: MedusaResponse) {
  if (process.env.NODE_ENV === "production") throw new MedusaError(MedusaError.Types.NOT_FOUND, "Not found")
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const { data } = await query.graph({ entity: "order", fields: ["id", "metadata", "email", "fulfillments.id", "fulfillments.shipped_at", "fulfillments.delivered_at", "fulfillments.canceled_at", "fulfillments.labels.tracking_number", "fulfillments.labels.tracking_url"], filters: { id: req.params.id } })
  const order = data[0]
  if (order?.metadata?.shipping_demo !== "dhl-official" || order.email !== "dhl-native-demo@example.invalid") throw new MedusaError(MedusaError.Types.NOT_FOUND, "Not found")
  res.setHeader("Cache-Control", "no-store")
  res.json({ shipments: (order.fulfillments || []).filter((item) => item && item.shipped_at && !item.canceled_at).map((item) => ({ id: item!.id, shipped_at: item!.shipped_at, delivered_at: item!.delivered_at, labels: item!.labels || [], official_demo: true })) })
}
