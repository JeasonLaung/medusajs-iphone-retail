export const faultIds = ["bright", "pixels", "lines"]
export const glassIds = ["cracked", "chipped", "scratched"]
export const gradeIds = ["damaged", "visible", "some", "minimal", "none"]
export type RecycledAssessment = {
  priceAgreed?: boolean
  battery: string
  checks: (boolean | null)[]
  faults: string[]
  glass: string[]
  screen: string
  frame: string
  back: string
}
export const emptyAssessment = (): RecycledAssessment => ({ battery: "", checks: [null, null, null, null, null], faults: [], glass: [], screen: "", frame: "", back: "" })
export const skipScreen = (value: RecycledAssessment) => [...value.faults, ...value.glass].some((item) => item !== "none")
export const validBattery = (value: string) => value === "unknown" || /^\d{1,3}$/.test(value) && Number(value) >= 1 && Number(value) <= 100
export const validMulti = (values: string[], ids: string[]) => values.length > 0 && values.every((value) => [...ids, "none"].includes(value)) && (!values.includes("none") || values.length === 1)
export const eligible = (value: RecycledAssessment) => value.checks[0] !== false && value.checks[1] !== false
export const completeAssessment = (value: RecycledAssessment) => validBattery(value.battery) && eligible(value) && value.checks.length === 5 && value.checks.every((item) => typeof item === "boolean") && validMulti(value.faults, faultIds) && validMulti(value.glass, glassIds) && (skipScreen(value) || gradeIds.slice(1).includes(value.screen)) && gradeIds.includes(value.frame) && (value.frame === "damaged" || gradeIds.includes(value.back))

// Keep the assessment in the existing notes field without a schema migration.
// Versioned enum values remain independent of the operator's interface language.
export function assessmentNotes(assessment: RecycledAssessment, notes: string) {
  return JSON.stringify({ buyback_v1: { ...assessment, screen: skipScreen(assessment) ? "" : assessment.screen, back: assessment.frame === "damaged" ? "" : assessment.back }, notes })
}
export function readAssessment(notes: string): { assessment?: RecycledAssessment; notes: string } {
  try {
    const value = JSON.parse(notes)
    if (value && typeof value.notes === "string" && value.buyback_v1 && completeAssessment(value.buyback_v1)) return { assessment: value.buyback_v1, notes: value.notes }
  } catch { /* Existing free-text notes are preserved. */ }
  return { notes }
}
