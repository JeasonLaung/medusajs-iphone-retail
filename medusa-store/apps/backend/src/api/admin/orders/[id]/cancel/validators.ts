import { z } from "@medusajs/framework/zod"

export const CancelOrderBody = z.object({
  reason: z.string().trim().min(1, "ORDER_CANCEL_REASON_REQUIRED").max(1000, "ORDER_CANCEL_REASON_TOO_LONG"),
})
export type CancelOrderBody = z.infer<typeof CancelOrderBody>
