import type { MedusaContainer } from "@medusajs/framework/types"
import { Modules } from "@medusajs/framework/utils"
import { z } from "@medusajs/framework/zod"

const note = z.object({ en: z.string().trim().max(200), zh: z.string().trim().max(200) })
export const BuyPoliciesBody = z.strictObject({
  warranty_months: z.number().int().min(0).max(120),
  return_days: z.number().int().min(0).max(365),
  warranty_note: note,
  return_note: note,
})
export type BuyPolicies = z.infer<typeof BuyPoliciesBody>
export const defaultBuyPolicies: BuyPolicies = {
  warranty_months: 24,
  return_days: 14,
  warranty_note: { en: "", zh: "" },
  return_note: { en: "", zh: "" },
}

export async function buyPolicySettings(container: MedusaContainer) {
  const [store] = await container.resolve(Modules.STORE).listStores({}, { take: 1 })
  const parsed = BuyPoliciesBody.safeParse(store?.metadata?.buy_policies)
  return { store, policies: parsed.success ? parsed.data : defaultBuyPolicies }
}
