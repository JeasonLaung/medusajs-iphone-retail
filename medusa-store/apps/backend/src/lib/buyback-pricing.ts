import type { RecycledAssessment } from "./recycled-assessment"

export const visualConditions = ["LIKE_NEW", "ALMOST_NEW", "GOOD", "MODERATE"] as const
export const pricingFaults = ["BROKEN", "BROKEN_SCREEN", "BATTERY_ISSUE", "BROKEN_FRAME", "BROKEN_BACK_CAMERA"] as const
export type VisualCondition = typeof visualConditions[number]
export type PricingFault = typeof pricingFaults[number]
export type PricingRules = {
  version: 1; enabled: boolean
  bases: Record<VisualCondition, number>
  fault_costs: Record<PricingFault, number>
  damage_fee: number; floor: number; battery_threshold: number; unknown_battery_issue: boolean
  source?: { name: string; country: string; fetched_at: string }
}
export const roundPrice = (value: number) => Math.round((value + Number.EPSILON) * 100) / 100
export const normalizeCapacity = (value: string) => value.replace(/\s+/g, "").toUpperCase().replace(/^1024GB$/, "1TB").replace(/^2048GB$/, "2TB")

export function calculatePrice(rules: PricingRules, visual: VisualCondition, faults: PricingFault[]) {
  const unique = [...new Set(faults)]
  const deductions = unique.reduce((sum, fault) => sum + rules.fault_costs[fault], 0)
  return roundPrice(Math.max(rules.floor, rules.bases[visual] - (unique.length ? rules.damage_fee : 0) - deductions))
}

// These are our explicit questionnaire policies, not claims about Swappie's UI mapping.
export function assessmentPrice(rules: PricingRules | null | undefined, assessment: Partial<RecycledAssessment>, legacyAmount: number | null = null) {
  if (assessment.checks?.[0] === false || assessment.checks?.[1] === false) return { amount: null, status: "ineligible" as const, visual: null, faults: [], provisional: false }
  if (!rules) return { amount: legacyAmount, status: legacyAmount == null ? "unconfigured" as const : "legacy" as const, visual: null, faults: [], provisional: false }
  if (!rules.enabled) return { amount: null, status: "disabled" as const, visual: null, faults: [], provisional: false }
  const faults = new Set<PricingFault>()
  if (assessment.checks?.slice(2).includes(false)) faults.add("BROKEN")
  const screenBroken = [...(assessment.faults || []), ...(assessment.glass || [])].some((value) => value !== "none")
  if (screenBroken) faults.add("BROKEN_SCREEN")
  if (assessment.frame === "damaged" || assessment.frame !== "damaged" && assessment.back === "damaged") faults.add("BROKEN_FRAME")
  if (assessment.battery === "unknown" ? rules.unknown_battery_issue : Boolean(assessment.battery) && Number(assessment.battery) < rules.battery_threshold) faults.add("BATTERY_ISSUE")
  const grades = [screenBroken ? "" : assessment.screen, assessment.frame, assessment.frame === "damaged" ? "" : assessment.back]
  const visual: VisualCondition = grades.includes("visible") ? "MODERATE" : grades.includes("some") ? "GOOD" : grades.includes("minimal") ? "ALMOST_NEW" : "LIKE_NEW"
  const provisional = !assessment.battery || assessment.battery === "unknown" || !assessment.checks || assessment.checks.length !== 5 || assessment.checks.some((v) => v == null) || !assessment.faults?.length || !assessment.glass?.length || !screenBroken && !assessment.screen || !assessment.frame || assessment.frame !== "damaged" && !assessment.back
  return { amount: calculatePrice(rules, visual, [...faults]), status: "estimated" as const, visual, faults: [...faults], provisional: Boolean(provisional) }
}
