import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import type { ModerateReview } from "../../../review-validators"
import { moderateProductReviewWorkflow } from "../../../../workflows/product-reviews"

export async function POST(req: AuthenticatedMedusaRequest<ModerateReview>, res: MedusaResponse) {
  const { result } = await moderateProductReviewWorkflow(req.scope).run({ input: { id: req.params.id, ...req.validatedBody } })
  res.json({ review: result })
}
