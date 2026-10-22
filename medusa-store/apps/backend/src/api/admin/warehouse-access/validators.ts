import { z } from "@medusajs/framework/zod"

export const WarehouseBindingBody = z.strictObject({ user_id: z.string().min(1).max(100), scope: z.enum(["platform", "all", "bound"]), location_ids: z.array(z.string().min(1).max(100)).max(100).refine((ids) => new Set(ids).size === ids.length, "Duplicate locations") })
