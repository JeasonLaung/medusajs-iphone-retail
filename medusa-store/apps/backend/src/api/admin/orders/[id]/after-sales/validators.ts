import { z } from "@medusajs/framework/zod"

export const UpdateAfterSales = z.object({
  request_id: z.string().min(1),
  status: z.enum(["processing", "completed", "rejected"]),
}).strict()

export type UpdateAfterSalesBody = z.infer<typeof UpdateAfterSales>
