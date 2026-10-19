import type { AuthenticatedMedusaRequest, MedusaStoreRequest, MedusaResponse } from "@medusajs/framework/http"
import { PRODUCT_REVIEW_MODULE } from "../../../modules/product-review"
import ProductReviewModuleService from "../../../modules/product-review/service"
import { createProductReviewWorkflow } from "../../../workflows/product-reviews"
import { CreateReview, ReviewListQuery } from "../../review-validators"
import { requireReviewProduct } from "../../review-product"

export async function GET(req: MedusaStoreRequest, res: MedusaResponse) {
  const { product_id, offset, limit } = ReviewListQuery.parse(req.validatedQuery)
  await requireReviewProduct(req.scope, product_id, req.publishable_key_context?.sales_channel_ids || [])
  const service: ProductReviewModuleService = req.scope.resolve(PRODUCT_REVIEW_MODULE)
  const filters = { product_id, status: "approved" as const }
  const [[reviews, count], ...ratings] = await Promise.all([
    service.listAndCountProductReviews(filters, {
      select: ["id", "author", "rating", "title", "content", "created_at", "verified_purchase"],
      take: limit, skip: offset, order: { created_at: "DESC", id: "DESC" },
    }),
    ...[1, 2, 3, 4, 5].map((rating) => service.listAndCountProductReviews({ ...filters, rating }, { take: 1, select: ["id"] })),
  ])
  const distribution = ratings.map(([, total], index) => ({ rating: index + 1, count: total }))
  const average = count ? distribution.reduce((sum, item) => sum + item.rating * item.count, 0) / count : 0
  res.json({ reviews, count, offset, limit, average, distribution })
}

export async function POST(req: AuthenticatedMedusaRequest<CreateReview>, res: MedusaResponse) {
  const { result } = await createProductReviewWorkflow(req.scope).run({ input: {
    ...req.validatedBody,
    customer_id: req.auth_context.actor_id,
    sales_channel_ids: req.publishable_key_context?.sales_channel_ids || [],
  } })
  res.status(201).json({ review: result })
}
