import type { MedusaContainer } from "@medusajs/framework/types"
import { Modules } from "@medusajs/framework/utils"
import { z } from "@medusajs/framework/zod"

const id = z
  .string()
  .trim()
  .regex(/^([a-f\d]{24})?$/i)
export const TrustpilotBody = z
  .strictObject({
    enabled: z.boolean(),
    profile_url: z
      .string()
      .trim()
      .max(300)
      .refine(
        (value) =>
          !value ||
          /^https:\/\/(?:www\.)?trustpilot\.com\/review\/[a-z\d.-]+\/?$/i.test(
            value,
          ),
      ),
    business_unit_id: id,
    summary_template_id: id,
    reviews_template_id: id,
    summary_height: z.number().int().min(40).max(300),
    reviews_height: z.number().int().min(140).max(1200),
  })
  .refine(
    (value) =>
      !value.enabled ||
      Boolean(
        value.profile_url &&
        value.business_unit_id &&
        value.summary_template_id &&
        value.reviews_template_id,
      ),
    { message: "TRUSTPILOT_CONFIGURATION_REQUIRED" },
  )

export type TrustpilotSettings = z.infer<typeof TrustpilotBody>
export const defaultTrustpilot: TrustpilotSettings = {
  enabled: false,
  profile_url: "",
  business_unit_id: "",
  summary_template_id: "",
  reviews_template_id: "",
  summary_height: 80,
  reviews_height: 300,
}

export async function trustpilotSettings(container: MedusaContainer) {
  const [store] = await container
    .resolve(Modules.STORE)
    .listStores({}, { take: 1 })
  const parsed = TrustpilotBody.safeParse(store?.metadata?.trustpilot)
  return { store, settings: parsed.success ? parsed.data : defaultTrustpilot }
}
