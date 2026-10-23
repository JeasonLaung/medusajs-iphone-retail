import { z } from "@medusajs/framework/zod"
export const SeoCommandBody = z.object({
  action: z.enum(["save", "delete", "scan", "run", "indexnow", "google-test"]),
  resource: z.enum(["settings", "page-metas", "crawler-sources", "providers", "citation-monitors"]).optional(),
  id: z.string().max(200).optional(), data: z.record(z.string(), z.unknown()).optional(),
})
export type SeoCommandBody = z.infer<typeof SeoCommandBody>
