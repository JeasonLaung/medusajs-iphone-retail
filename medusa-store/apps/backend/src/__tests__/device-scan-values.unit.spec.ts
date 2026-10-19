import { classifyDeviceScans, mergeDeviceScans } from "../admin/lib/device-scan-values"

const serial = "DEVICECHECK123"
const imei = "012345678901234"
const eid = "89049032000000000000000000000001"
const empty = { serial: "", imei: "", imei2: "" }

describe("multiple device barcodes", () => {
  it("fills serial and both radio fields in one atomic result, independent of scan order", () => {
    expect(mergeDeviceScans(empty, [imei, serial, eid, imei])).toEqual({ serial, imei, imei2: eid, conflict: false })
    expect(mergeDeviceScans(empty, [eid, serial, imei])).toEqual({ serial, imei: eid, imei2: imei, conflict: false })
  })
  it("recognizes labeled multiline payloads and preserves long identifiers as strings", () => {
    expect(classifyDeviceScans([`S/N: ${serial}\nIMEI1: ${imei}\nEID: ${eid}`])).toEqual({ serials: [serial], radios: [imei, eid], unknown: [] })
  })
  it("preserves existing values and does not duplicate repeated detections", () => {
    expect(mergeDeviceScans({ serial, imei, imei2: "" }, [serial, imei, eid])).toEqual({ serial, imei, imei2: eid, conflict: false })
    expect(mergeDeviceScans({ serial: "OTHER", imei, imei2: eid }, [serial, "112345678901234"]))
      .toEqual({ serial: "OTHER", imei, imei2: eid, conflict: true })
  })
  it("does not guess between multiple serial candidates or treat invalid radio lengths as serials", () => {
    expect(mergeDeviceScans(empty, [serial, "OTHER", imei])).toEqual({ serial: "", imei, imei2: "", conflict: true })
    expect(classifyDeviceScans(["1".repeat(20)])).toEqual({ serials: [], radios: [], unknown: ["1".repeat(20)] })
  })
})
