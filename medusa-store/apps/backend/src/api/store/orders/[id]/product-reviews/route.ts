import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { requireOwnedReviewOrder, isReviewOrderEligible } from "../../../../../lib/review-orders"
import { PRODUCT_REVIEW_MODULE } from "../../../../../modules/product-review"
import ProductReviewModuleService from "../../../../../modules/product-review/service"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const customer_id = req.auth_context.actor_id
  const order = await requireOwnedReviewOrder(req.scope, req.params.id, customer_id)
  const productIds = [...new Set(order.items?.flatMap((item) => item?.product_id ? [item.product_id] : []) || [])]
  const service: ProductReviewModuleService = req.scope.resolve(PRODUCT_REVIEW_MODULE)
  const reviews = productIds.length ? await service.listProductReviews({ customer_id, product_id: productIds }, {
    select: ["product_id", "status", "verified_purchase"], take: productIds.length,
  }) : []
  res.json({ eligible: isReviewOrderEligible(order), reviews })
}
