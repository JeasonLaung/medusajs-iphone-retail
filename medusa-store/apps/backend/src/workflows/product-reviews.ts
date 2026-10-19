import {
  createStep,
  createWorkflow,
  StepResponse,
  WorkflowResponse,
} from "@medusajs/framework/workflows-sdk"
import { MedusaError, Modules } from "@medusajs/framework/utils"
import { PRODUCT_REVIEW_MODULE } from "../modules/product-review"
import ProductReviewModuleService from "../modules/product-review/service"
import { requireReviewProduct } from "../api/review-product"
import type { CreateReview, ModerateReview } from "../api/review-validators"
import {
  isReviewOrderEligible,
  requireOwnedReviewOrder,
} from "../lib/review-orders"

type CreateInput = CreateReview & {
  customer_id: string
  sales_channel_ids: string[]
}
const createReviewStep = createStep(
  "create-review",
  async (input: CreateInput, { container }) => {
    await requireReviewProduct(
      container,
      input.product_id,
      input.sales_channel_ids
    )
    if (input.order_id) {
      const order = await requireOwnedReviewOrder(
        container,
        input.order_id,
        input.customer_id
      )
      if (
        !isReviewOrderEligible(order) ||
        !order.items?.some((item) => item?.product_id === input.product_id)
      ) {
        throw new MedusaError(
          MedusaError.Types.NOT_ALLOWED,
          "review.order_ineligible"
        )
      }
    }
    const service: ProductReviewModuleService = container.resolve(
      PRODUCT_REVIEW_MODULE
    )
    const existing = await service.listProductReviews(
      { product_id: input.product_id, customer_id: input.customer_id },
      { take: 1 }
    )
    if (existing.length)
      throw new MedusaError(
        MedusaError.Types.CONFLICT,
        "review.already_submitted"
      )
    const customer = await container
      .resolve(Modules.CUSTOMER)
      .retrieveCustomer(input.customer_id)
    // Only a first name is public. Email and customer identifiers never leave the store API.
    const review = await service.createProductReviews({
      product_id: input.product_id,
      customer_id: input.customer_id,
      order_id: input.order_id || null,
      verified_purchase: !!input.order_id,
      author: customer.first_name?.trim().slice(0, 60) || "",
      rating: input.rating,
      title: input.title,
      content: input.content,
      status: "pending",
    })
    return new StepResponse({ id: review.id, status: review.status }, review.id)
  },
  async (id, { container }) => {
    if (id)
      await (
        container.resolve(PRODUCT_REVIEW_MODULE) as ProductReviewModuleService
      ).deleteProductReviews(id)
  }
)

export const createProductReviewWorkflow = createWorkflow(
  "create-product-review",
  function (input: CreateInput) {
    return new WorkflowResponse(createReviewStep(input))
  }
)

type ModerateInput = ModerateReview & { id: string }
const moderateReviewStep = createStep(
  "moderate-review",
  async (input: ModerateInput, { container }) => {
    const service: ProductReviewModuleService = container.resolve(
      PRODUCT_REVIEW_MODULE
    )
    const previous = await service.retrieveProductReview(input.id)
    const review = await service.updateProductReviews(input)
    return new StepResponse(review, {
      id: previous.id,
      status: previous.status,
    })
  },
  async (previous, { container }) => {
    if (previous)
      await (
        container.resolve(PRODUCT_REVIEW_MODULE) as ProductReviewModuleService
      ).updateProductReviews(previous)
  }
)

export const moderateProductReviewWorkflow = createWorkflow(
  "moderate-product-review",
  function (input: ModerateInput) {
    return new WorkflowResponse(moderateReviewStep(input))
  }
)
