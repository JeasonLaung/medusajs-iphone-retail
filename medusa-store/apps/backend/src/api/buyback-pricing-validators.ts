import { z } from "@medusajs/framework/zod"
import { pricingFaults, visualConditions } from "../lib/buyback-pricing"

const money = z.number().finite().min(0).max(1000000).refine((value) => Math.abs(value * 100 - Math.round(value * 100)) < 0.000001)
export const PricingRulesBody = z.strictObject({
  version: z.literal(1), enabled: z.boolean(),
  bases: z.record(z.enum(visualConditions), money), fault_costs: z.record(z.enum(pricingFaults), money),
  damage_fee: money, floor: money, battery_threshold: z.number().int().min(1).max(100), unknown_battery_issue: z.boolean(),
  source: z.strictObject({ name: z.string().max(40), country: z.string().max(4), fetched_at: z.string().max(40) }).optional(),
}).refine((rules) => rules.bases.LIKE_NEW >= rules.bases.ALMOST_NEW && rules.bases.ALMOST_NEW >= rules.bases.GOOD && rules.bases.GOOD >= rules.bases.MODERATE && rules.bases.MODERATE >= rules.floor, { message: "BUYBACK_INVALID_PRICE_ORDER" })
export const BuybackCatalogCommand = z.discriminatedUnion("action", [
  z.strictObject({ action: z.literal("save"), product_id: z.string().min(1).max(100), capacity: z.string().regex(/^\d+(GB|TB)$/), rules: PricingRulesBody }),
  z.strictObject({ action: z.literal("reference"), keys: z.array(z.string().max(150)).min(1).max(200), only_missing: z.boolean().default(true), percentage: z.number().finite().min(-100).max(500).default(0), adjustment: z.number().finite().min(-100000).max(100000).default(0) }),
  z.strictObject({ action: z.literal("adjust"), keys: z.array(z.string().max(150)).min(1).max(200), percentage: z.number().finite().min(-100).max(500), adjustment: z.number().finite().min(-100000).max(100000) }),
])
export type BuybackCatalogCommand = z.infer<typeof BuybackCatalogCommand>
const partialAssessment = z.strictObject({ battery: z.string().max(7).refine((value) => value === "" || value === "unknown" || /^\d{1,3}$/.test(value) && Number(value) >= 1 && Number(value) <= 100), checks: z.array(z.boolean().nullable()).length(5), faults: z.array(z.enum(["bright", "pixels", "lines", "none"])).max(4), glass: z.array(z.enum(["cracked", "chipped", "scratched", "none"])).max(4), screen: z.enum(["", "visible", "some", "minimal", "none"]), frame: z.enum(["", "damaged", "visible", "some", "minimal", "none"]), back: z.enum(["", "damaged", "visible", "some", "minimal", "none"]) })
export const BuybackEstimateBody = z.strictObject({ product_id: z.string().min(1).max(100), capacity: z.string().regex(/^\d+(GB|TB)$/), assessment: partialAssessment })
