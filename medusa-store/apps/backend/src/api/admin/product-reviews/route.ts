import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { PRODUCT_REVIEW_MODULE } from "../../../modules/product-review"
import ProductReviewModuleService from "../../../modules/product-review/service"
import { ReviewListQuery } from "../../review-validators"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const { product_id, offset, limit } = ReviewListQuery.parse(req.validatedQuery)
  const service: ProductReviewModuleService = req.scope.resolve(PRODUCT_REVIEW_MODULE)
  const [reviews, count] = await service.listAndCountProductReviews({ product_id }, {
    take: limit, skip: offset, order: { created_at: "DESC", id: "DESC" },
    select: ["id", "author", "rating", "title", "content", "status", "created_at", "order_id", "verified_purchase"],
  })
  res.json({ reviews, count, offset, limit })
}
