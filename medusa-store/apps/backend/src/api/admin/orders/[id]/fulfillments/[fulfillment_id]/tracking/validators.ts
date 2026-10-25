import { z } from "@medusajs/framework/zod"

export const UpdateShipmentTracking = z.object({
  labels: z.array(z.object({
    id: z.string().min(1).max(100).optional(),
    tracking_number: z.string().trim().min(1).max(200),
    tracking_url: z.union([z.literal(""), z.url({ protocol: /^https?$/ })]).default(""),
  })).min(1).max(20),
})
export type UpdateShipmentTrackingBody = z.infer<typeof UpdateShipmentTracking>
