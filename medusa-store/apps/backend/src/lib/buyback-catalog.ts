import type { MedusaContainer } from "@medusajs/framework/types"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import reference from "./data/swappie-buyback-reference.json"
import { BUYBACK_MODULE } from "../modules/buyback"
import type BuybackModuleService from "../modules/buyback/service"
import { normalizeCapacity, type PricingRules } from "./buyback-pricing"

export function referenceRules(model: string, capacity: string): PricingRules | null {
  const entry = reference.configurations.find((entry) => entry.model.toLowerCase() === model.trim().toLowerCase() && entry.capacity === normalizeCapacity(capacity))
  if (!entry) return null
  return { version: 1, enabled: true, bases: { LIKE_NEW: entry.bases.LIKE_NEW, ALMOST_NEW: entry.bases.ALMOST_NEW, GOOD: entry.bases.GOOD, MODERATE: entry.bases.MODERATE }, fault_costs: entry.fault_costs, damage_fee: entry.damage_fee!, floor: entry.floor, battery_threshold: 80, unknown_battery_issue: true, source: { name: "Swappie", country: reference.country, fetched_at: entry.fetched_at } }
}

export async function buybackCatalog(container: MedusaContainer) {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const products: { id: string; title: string; status?: string; options?: { title: string; values?: { value: string }[] }[] }[] = []
  let skip = 0
  while (true) {
    const { data } = await query.graph({ entity: "product", fields: ["id", "title", "status", "options.title", "options.values.value"], pagination: { skip, take: 100 } })
    products.push(...data.filter((product) => /^iPhone\b/i.test(product.title)))
    if (data.length < 100) break
    skip += data.length
  }
  const prices = await container.resolve<BuybackModuleService>(BUYBACK_MODULE).listBuybackPrices({}, { take: null })
  const entries = products.flatMap((product) => {
    const capacities = [...new Set(product.options?.filter((option) => /storage|capacity|memory|容量/i.test(option.title)).flatMap((option) => option.values?.map((value) => normalizeCapacity(value.value)) || []) || [])].filter((capacity) => /^\d+(GB|TB)$/.test(capacity))
    return capacities.map((capacity) => {
      const price = prices.find((price) => price.product_id === product.id && normalizeCapacity(price.capacity) === capacity)
      return { key: `${product.id}:${capacity}`, product_id: product.id, model: product.title, capacity, product_status: product.status, amount: price ? Number(price.amount) : null, rules: (price?.pricing_rules as PricingRules | null) ?? null, reference: referenceRules(product.title, capacity) }
    })
  }).sort((a, b) => b.model.localeCompare(a.model, "en", { numeric: true }) || parseInt(a.capacity) * (a.capacity.endsWith("TB") ? 1024 : 1) - parseInt(b.capacity) * (b.capacity.endsWith("TB") ? 1024 : 1))
  return { entries, source: { name: "Swappie", country: reference.country, currency: reference.currency, fetched_at: reference.fetched_at, model_count: reference.model_count, configuration_count: reference.configurations.length, row_count: reference.row_count, additive_matches: reference.additive_matches } }
}
