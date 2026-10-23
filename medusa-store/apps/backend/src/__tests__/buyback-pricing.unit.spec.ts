import reference from "../lib/data/swappie-buyback-reference.json"
import { assessmentPrice, calculatePrice, normalizeCapacity, type PricingRules, type PricingFault, type VisualCondition } from "../lib/buyback-pricing"
import { referenceRules } from "../lib/buyback-catalog"
import { BuybackCatalogCommand, BuybackEstimateBody, PricingRulesBody } from "../api/buyback-pricing-validators"

const rules = () => referenceRules("iPhone 14", "256GB")!
const healthy = { battery: "90", checks: [true, true, true, true, true], faults: ["none"], glass: ["none"], screen: "none", frame: "none", back: "none" }
describe("buyback estimates from the captured Swappie matrix", () => {
  it("reproduces all 10,720 captured combination prices, including floor clipping", () => {
    let count = 0
    for (const entry of reference.configurations) {
      // SEALED_BOX is verified against source data, but excluded from our questionnaire.
      const pricing = { ...rules(), bases: entry.bases, fault_costs: entry.fault_costs, damage_fee: entry.damage_fee!, floor: entry.floor } as PricingRules
      for (const [key, row] of Object.entries(entry.matrix)) {
        const [visual, faults] = key.split(":")
        expect(calculatePrice(pricing, visual as VisualCondition, faults ? faults.split(",") as PricingFault[] : [])).toBe(row.amount)
        count++
      }
    }
    expect(count).toBe(10720)
  })
  it("charges the shared damage fee and each fault category once", () => {
    expect(assessmentPrice(rules(), healthy).amount).toBe(294)
    const damaged = { ...healthy, faults: ["bright", "pixels", "lines"], glass: ["cracked", "chipped"], battery: "79" }
    expect(assessmentPrice(rules(), damaged)).toMatchObject({ amount: 152.02, faults: ["BROKEN_SCREEN", "BATTERY_ISSUE"], provisional: false })
    expect(calculatePrice(rules(), "LIKE_NEW", ["BROKEN_SCREEN", "BROKEN_SCREEN"])).toBe(175.02)
    expect(assessmentPrice(rules(), { ...damaged, frame: "damaged", checks: [true, true, false, false, false] }).amount).toBe(25)
  })
  it("uses worst undamaged appearance without charging damaged parts twice", () => {
    expect(assessmentPrice(rules(), { ...healthy, screen: "minimal", frame: "some", back: "visible" })).toMatchObject({ amount: 236, visual: "MODERATE" })
    expect(assessmentPrice(rules(), { ...healthy, faults: ["pixels"], screen: "visible" })).toMatchObject({ visual: "LIKE_NEW", amount: 175.02 })
    expect(assessmentPrice(rules(), { ...healthy, frame: "damaged", back: "visible" })).toMatchObject({ visual: "LIKE_NEW", faults: ["BROKEN_FRAME"], amount: 181.02 })
    expect(assessmentPrice(rules(), { ...healthy, back: "damaged" })).toMatchObject({ visual: "LIKE_NEW", faults: ["BROKEN_FRAME"] })
  })
  it("honors battery threshold, unknown policy, partial answers and ineligibility", () => {
    expect(assessmentPrice(rules(), { ...healthy, battery: "80" }).amount).toBe(294)
    expect(assessmentPrice(rules(), { ...healthy, battery: "79" }).amount).toBe(257.02)
    expect(assessmentPrice(rules(), { ...healthy, battery: "unknown" })).toMatchObject({ amount: 257.02, provisional: true })
    expect(assessmentPrice({ ...rules(), unknown_battery_issue: false }, { ...healthy, battery: "unknown" })).toMatchObject({ amount: 294, provisional: true })
    expect(assessmentPrice(rules(), {})).toMatchObject({ amount: 294, provisional: true })
    expect(assessmentPrice(rules(), { ...healthy, checks: [false, true, true, true, true] })).toMatchObject({ amount: null, status: "ineligible" })
    expect(assessmentPrice({ ...rules(), enabled: false }, healthy)).toMatchObject({ amount: null, status: "disabled" })
  })
  it("preserves legacy references, does not invent missing storage prices, normalizes TB", () => {
    expect(assessmentPrice(null, healthy, 123)).toMatchObject({ amount: 123, status: "legacy" })
    expect(assessmentPrice(null, healthy)).toMatchObject({ amount: null, status: "unconfigured" })
    expect(referenceRules("iPhone 14", "128GB")).toBeNull()
    expect(normalizeCapacity("1024 GB")).toBe("1TB")
    expect(referenceRules("iPhone 15 Pro", "1024GB")).toEqual(referenceRules("iPhone 15 Pro", "1TB"))
  })
  it("validates price order, finite money, thresholds and estimate input", () => {
    expect(PricingRulesBody.safeParse(rules()).success).toBe(true)
    expect(PricingRulesBody.safeParse({ ...rules(), floor: 1000 }).success).toBe(false)
    expect(PricingRulesBody.safeParse({ ...rules(), damage_fee: -1 }).success).toBe(false)
    expect(PricingRulesBody.safeParse({ ...rules(), battery_threshold: 101 }).success).toBe(false)
    expect(BuybackCatalogCommand.safeParse({ action: "adjust", keys: ["p:256GB"], percentage: -101, adjustment: 0 }).success).toBe(false)
    const input = { product_id: "p", capacity: "256GB", assessment: healthy }
    expect(BuybackEstimateBody.safeParse(input).success).toBe(true)
    expect(BuybackEstimateBody.safeParse({ ...input, amount: 999 }).success).toBe(false)
    expect(BuybackEstimateBody.safeParse({ ...input, assessment: { ...healthy, battery: "101" } }).success).toBe(false)
  })
})
