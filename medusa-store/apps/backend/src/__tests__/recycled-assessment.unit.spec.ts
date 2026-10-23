import { IntakeDevice } from "../api/admin/devices/validators"
import { assessmentNotes, completeAssessment, emptyAssessment, readAssessment, type RecycledAssessment } from "../admin/lib/recycled-assessment"

const completed = (): RecycledAssessment => ({ battery: "unknown", checks: [true, true, false, true, true], faults: ["none"], glass: ["none"], screen: "minimal", frame: "some", back: "some", priceAgreed: false })

describe("manual recycled phone assessment", () => {
  it("requires completed answers and rejects ineligible power/network checks", () => {
    expect(completeAssessment(emptyAssessment())).toBe(false)
    expect(completeAssessment(completed())).toBe(true)
    expect(completeAssessment({ ...completed(), checks: [false, true, true, true, true] })).toBe(false)
    expect(completeAssessment({ ...completed(), checks: [true, false, true, true, true] })).toBe(false)
    for (const battery of ["0", "101", "87.5", "-1"]) expect(completeAssessment({ ...completed(), battery })).toBe(false)
    expect(completeAssessment({ ...completed(), battery: "87" })).toBe(true)
  })
  it("follows the storefront's screen and damaged-frame skip rules without saving stale answers", () => {
    const value = { ...completed(), faults: ["bright"], frame: "damaged" }
    expect(completeAssessment(value)).toBe(true)
    const saved = readAssessment(assessmentNotes(value, ""))
    expect(saved.assessment?.screen).toBe("")
    expect(saved.assessment?.back).toBe("")
    expect(completeAssessment({ ...completed(), faults: ["bright", "none"] })).toBe(false)
  })
  it("round-trips multilingual notes and preserves existing plain text or unrelated JSON", () => {
    const notes = "用户手机\nCustomer phone"
    expect(readAssessment(assessmentNotes(completed(), notes))).toEqual({ assessment: completed(), notes })
    for (const legacy of [notes, '{"notes":"other data"}', '{"buyback_v1":{},"notes":"x"}']) expect(readAssessment(legacy)).toEqual({ notes: legacy })
  })
  it("submits manual identity and assessment through the existing intake contract", () => {
    const parsed = IntakeDevice.parse({ category: "recycled", source_id: "source-test", variant_id: "variant-test", serial_number: "manualsn", imei: "123456789012345", notes: assessmentNotes(completed(), "Customer supplied phone") })
    expect(parsed.serial_number).toBe("MANUALSN")
    expect(parsed.category).toBe("recycled")
    expect(readAssessment(parsed.notes).assessment?.battery).toBe("unknown")
    expect(parsed.photos).toEqual([])
  })
})
