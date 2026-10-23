import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { acquireLockStep, releaseLockStep } from "@medusajs/medusa/core-flows"
import { BUYBACK_MODULE } from "../modules/buyback"
import type BuybackModuleService from "../modules/buyback/service"
import type { BuybackCatalogCommand } from "../api/buyback-pricing-validators"
import { PricingRulesBody } from "../api/buyback-pricing-validators"
import { adminBuybackAccess, assertBuyback } from "../lib/buyback"
import { buybackCatalog } from "../lib/buyback-catalog"
import { normalizeCapacity, roundPrice, type PricingRules } from "../lib/buyback-pricing"

type Input = { actor_id: string; command: BuybackCatalogCommand }
type Previous = { id: string; amount: number; pricing_rules: Record<string, unknown> | null }
const updateCatalog = createStep("update-buyback-catalog", async ({ actor_id, command }: Input, { container }) => {
  await adminBuybackAccess(container, actor_id)
  const service = container.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const { entries } = await buybackCatalog(container)
  const selected = command.action === "save" ? entries.filter((entry) => entry.product_id === command.product_id && entry.capacity === normalizeCapacity(command.capacity)) : entries.filter((entry) => command.keys.includes(entry.key))
  assertBuyback(selected.length > 0 && (command.action === "save" || selected.length === new Set(command.keys).size), "BUYBACK_INVALID_MODEL")
  const updates = selected.flatMap((entry) => {
    let rules: PricingRules | null = command.action === "save" ? command.rules : command.action === "reference" ? entry.reference : entry.rules
    if (command.action === "reference" && command.only_missing && entry.amount != null) return []
    if (!rules) return []
    rules = structuredClone(rules)
    if (command.action !== "save") {
      // Adjust the whole curve: scale base/repair costs/fee/floor; offset bases and floor.
      const factor = 1 + command.percentage / 100
      for (const visual of Object.keys(rules.bases)) rules.bases[visual] = roundPrice(Math.max(0, rules.bases[visual] * factor + command.adjustment))
      for (const fault of Object.keys(rules.fault_costs)) rules.fault_costs[fault] = roundPrice(rules.fault_costs[fault] * factor)
      rules.damage_fee = roundPrice(rules.damage_fee * factor)
      rules.floor = roundPrice(Math.max(0, rules.floor * factor + command.adjustment))
    }
    assertBuyback(PricingRulesBody.safeParse(rules).success, "BUYBACK_INVALID_PRICE_ORDER")
    return [{ entry, rules }]
  })
  const previous: Previous[] = [], created: string[] = []
  async function restore() {
    for (const row of previous) await service.updateBuybackPrices(row)
    if (created.length) await service.deleteBuybackPrices(created)
  }
  try {
    for (const { entry, rules } of updates) {
      const [existing] = await service.listBuybackPrices({ product_id: entry.product_id, capacity: entry.capacity })
      const data = { amount: rules.bases.LIKE_NEW, pricing_rules: rules as unknown as Record<string, unknown> }
      if (existing) { previous.push({ id: existing.id, amount: Number(existing.amount), pricing_rules: existing.pricing_rules }); await service.updateBuybackPrices({ id: existing.id, ...data }) }
      else { const row = await service.createBuybackPrices({ product_id: entry.product_id, capacity: entry.capacity, ...data }); created.push(row.id) }
    }
  } catch (error) { await restore(); throw error }
  return new StepResponse({ updated: updates.length, skipped: selected.length - updates.length }, { previous, created })
}, async (before: { previous: Previous[]; created: string[] } | undefined, { container }) => {
  if (!before) return
  const service = container.resolve<BuybackModuleService>(BUYBACK_MODULE)
  for (const row of before.previous) await service.updateBuybackPrices(row)
  if (before.created.length) await service.deleteBuybackPrices(before.created)
})
export const updateBuybackCatalogWorkflow = createWorkflow("update-buyback-catalog", (input: Input) => {
  acquireLockStep({ key: "buyback-price-catalog", timeout: 30, ttl: 120 })
  const result = updateCatalog(input)
  releaseLockStep({ key: "buyback-price-catalog" })
  return new WorkflowResponse(result)
})
