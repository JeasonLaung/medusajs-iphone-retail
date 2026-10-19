import type { MedusaContainer } from "@medusajs/framework/types"
import {
  ContainerRegistrationKeys,
  MedusaError,
} from "@medusajs/framework/utils"

export const reviewOrderFields = [
  "id",
  "display_id",
  "custom_display_id",
  "customer_id",
  "status",
  "created_at",
  "items.product_id",
  "items.title",
  "payment_collections.status",
]

export function isReviewOrderEligible(order: {
  status?: string
  payment_collections?: ({ status?: string } | null)[] | null
}) {
  return (
    order.status !== "canceled" &&
    order.status !== "draft" &&
    !!order.payment_collections?.length &&
    order.payment_collections.every(
      (collection) => collection?.status === "completed"
    )
  )
}

export async function requireOwnedReviewOrder(
  container: MedusaContainer,
  orderId: string,
  customerId: string
) {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data } = await query.graph({
    entity: "order",
    fields: reviewOrderFields,
    filters: { id: orderId, customer_id: customerId },
  })
  if (!data[0])
    throw new MedusaError(
      MedusaError.Types.NOT_FOUND,
      "review.order_unavailable"
    )
  return data[0]
}
