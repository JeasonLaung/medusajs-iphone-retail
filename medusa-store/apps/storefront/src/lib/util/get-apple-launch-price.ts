import type { HttpTypes } from "@medusajs/types"

// EUR prices supplied in the user's iPhone 13 references, by storage capacity.
const referencePrices: Record<string, Record<string, number>> = {
  "iphone 13": { "128gb": 869, "256gb": 979, "512gb": 1189 },
}

export function getAppleLaunchPrice(product: HttpTypes.StoreProduct, variant?: HttpTypes.StoreProductVariant, currency?: string) {
  const storageOption = product.options?.find((option) => /storage|capacity/i.test(option.title ?? ""))
  const storage = variant?.options?.find((option) => option.option_id === storageOption?.id)?.value
  if (!storage || !currency) return null
  const prices = product.metadata?.apple_launch_prices
  const byStorage = prices && typeof prices === "object" && !Array.isArray(prices) ? (prices as Record<string, unknown>)[storage] : undefined
  const raw = variant?.metadata?.apple_launch_price ?? byStorage
  if (raw && typeof raw === "object" && !Array.isArray(raw)) {
    const entry = raw as Record<string, unknown>
    if (typeof entry.amount === "number" && Number.isFinite(entry.amount) && entry.amount > 0 && entry.currency_code === currency) return entry.amount
  }
  return currency === "eur" ? referencePrices[(product.title ?? "").toLowerCase()]?.[storage.replace(/\s/g, "").toLowerCase()] ?? null : null
}
