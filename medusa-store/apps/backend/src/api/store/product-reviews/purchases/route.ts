import type {
  AuthenticatedMedusaRequest,
  MedusaResponse,
} from "@medusajs/framework/http"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { ReviewPurchasesQuery } from "../../../review-validators"
import {
  reviewOrderFields,
  isReviewOrderEligible,
} from "../../../../lib/review-orders"
import { PRODUCT_REVIEW_MODULE } from "../../../../modules/product-review"
import ProductReviewModuleService from "../../../../modules/product-review/service"

export async function GET(
  req: AuthenticatedMedusaRequest,
  res: MedusaResponse
) {
  const { product_id, offset } = ReviewPurchasesQuery.parse(req.validatedQuery)
  const customer_id = req.auth_context.actor_id
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const service: ProductReviewModuleService = req.scope.resolve(
    PRODUCT_REVIEW_MODULE
  )
  const [{ data: orders, metadata }, reviews] = await Promise.all([
    query.graph({
      entity: "order",
      fields: reviewOrderFields,
      filters: { customer_id },
      pagination: { take: 20, skip: offset, order: { created_at: "DESC" } },
    }),
    service.listProductReviews(
      { customer_id, product_id },
      { select: ["id", "status", "verified_purchase"], take: 1 }
    ),
  ])
  res.json({
    orders: orders.map((order) => ({
      id: order.id,
      number: order.custom_display_id || String(order.display_id),
      created_at: order.created_at,
      eligible:
        isReviewOrderEligible(order) &&
        !!order.items?.some((item) => item?.product_id === product_id),
    })),
    review: reviews[0] || null,
    count: metadata?.count || 0,
    offset,
    limit: 20,
  })
}
