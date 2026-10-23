import { z } from "@medusajs/framework/zod"

export const ReadNoticesBody = z.object({ id: z.string().min(1).max(200).optional() })
export type ReadNoticesBody = z.infer<typeof ReadNoticesBody>
