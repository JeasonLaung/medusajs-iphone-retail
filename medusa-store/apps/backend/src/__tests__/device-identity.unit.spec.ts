import { DeviceCommand, IntakeDevice } from "../api/admin/devices/validators"
import { assertDeviceIdentity } from "../modules/devices/identity"
import { matchesIdentity } from "../modules/devices/policy"
import { deviceIdentityFilters, isImeiOrEid } from "../lib/device-identity"

const imei = "012345678901234"
const eid = "89049032000000000000000000000001"
const imei2 = "112345678901234"
const product = { title: "iPhone 14 Pro Max" }

describe("device serial, IMEI and EID identities", () => {
  it("accepts 15-digit IMEIs and 32-digit EIDs while rejecting other lengths and letters", () => {
    expect(isImeiOrEid(imei)).toBe(true)
    expect(isImeiOrEid(eid)).toBe(true)
    for (const invalid of ["123", "1".repeat(20), "1".repeat(31), "1".repeat(33), "x".repeat(15)])
      expect(isImeiOrEid(invalid)).toBe(false)
  })

  it("accepts either slot for iPhone and keeps EIDs and leading zeros as text", () => {
    for (const fields of [{ imei }, { eid }, { imei2: imei }, { imei, eid }, { imei, imei2 }, { imei, imei2, eid }]) {
      const value = { serial_number: "serial123", variant_id: "variant", source_id: "source", ...fields }
      expect(() => assertDeviceIdentity(IntakeDevice.parse(value), product)).not.toThrow()
      const parsed = DeviceCommand.parse({ action: "order_assign", order_id: "order", item_id: "item", ...fields, serial_number: "serial123" })
      expect(parsed).toMatchObject(fields)
    }
    expect(() => assertDeviceIdentity({ serial_number: "serial" }, product)).toThrow(/至少一个/)
    expect(() => assertDeviceIdentity({ serial_number: "serial", imei, imei2: imei }, product)).toThrow(/不能相同/)
    expect(IntakeDevice.safeParse({ serial_number: "serial", variant_id: "variant", source_id: "source", imei2: "1".repeat(20) }).success).toBe(false)
  })

  it("rejects EIDs in IMEI slots and IMEIs in the EID slot", () => {
    for (const fields of [{ imei: eid }, { imei2: eid }, { eid: imei }]) {
      expect(IntakeDevice.safeParse({ serial_number: "SERIAL", variant_id: "v", source_id: "s", ...fields }).success).toBe(false)
      expect(() => assertDeviceIdentity({ serial_number: "SERIAL", ...fields }, product)).toThrow()
    }
  })
  it("matches all four identifiers and searches either number across both slots", () => {
    const device = { serial_number: "SERIAL123", imei, imei2, eid }
    for (const value of [" serial123 ", imei, imei2, eid]) expect(matchesIdentity(value, device)).toBe(true)
    expect(matchesIdentity("unrelated", device)).toBe(false)
    expect(matchesIdentity("", {})).toBe(false)
    expect(deviceIdentityFilters(device)).toEqual([
      { serial_number: "SERIAL123" },
      ...[imei, imei2, eid].flatMap(value => [{ imei: value }, { imei2: value }, { eid: value }]),
    ])
  })
})
