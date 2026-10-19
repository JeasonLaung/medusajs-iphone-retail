import { z } from "@medusajs/framework/zod"

export const ReviewListQuery = z.object({
  product_id: z.string().trim().min(1).max(255),
  offset: z.coerce.number().int().min(0).default(0),
  limit: z.coerce.number().int().min(1).max(20).default(5),
})
export const CreateReview = z.object({
  product_id: z.string().trim().min(1).max(255),
  order_id: z.string().trim().min(1).max(255).optional(),
  rating: z.number().int().min(1).max(5),
  title: z.string().trim().min(2).max(120),
  content: z.string().trim().min(10).max(3000),
})
export type CreateReview = z.infer<typeof CreateReview>
export const ReviewPurchasesQuery = z.object({
  product_id: z.string().trim().min(1).max(255),
  offset: z.coerce.number().int().min(0).default(0),
})
export const ModerateReview = z.object({
  status: z.enum(["pending", "approved", "rejected"]),
})
export type ModerateReview = z.infer<typeof ModerateReview>
