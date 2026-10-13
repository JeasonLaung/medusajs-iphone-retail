import { z } from "@medusajs/framework/zod"

export const CreateAfterSales = z.object({
  type: z.enum(["return", "exchange", "repair"]),
  reason: z.string().trim().min(10).max(2000),
  items: z.array(z.object({
    item_id: z.string().min(1),
    quantity: z.number().int().min(1).max(10000),
    device_ids: z.array(z.string().min(1).max(100)).min(1).max(100).optional(),
  }).strict()).min(1).max(100),
}).strict()

export type CreateAfterSalesBody = z.infer<typeof CreateAfterSales>
