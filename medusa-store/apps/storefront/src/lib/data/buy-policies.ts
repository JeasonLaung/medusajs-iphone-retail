import "server-only"
import { sdk } from "@lib/config"

export type BuyPolicies = {
  warranty_months: number
  return_days: number
  warranty_note: { en: string; zh: string }
  return_note: { en: string; zh: string }
}

export async function getBuyPolicies(): Promise<BuyPolicies | null> {
  try {
    const { policies } = await sdk.client.fetch<{ policies: BuyPolicies }>("/store/buy-policies", { cache: "no-store" })
    return policies
  } catch {
    return null
  }
}
