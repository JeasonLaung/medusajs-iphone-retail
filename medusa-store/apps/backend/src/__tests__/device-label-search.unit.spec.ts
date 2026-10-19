import { deviceLabelSearch } from "../admin/lib/device-label-search"

describe("SKU search from label text", () => {
  it("extracts product attributes without serial, radio identifiers or shop text", () => {
    expect(deviceLabelSearch("CS\nS/N: Q7M2K9V4XP\niPhone 15\n128 GB Yellow\nIMEI: 490154203237518\nEID: 89049032000000000000000000000001"))
      .toBe("iphone 15 128 yellow")
  })
  it("handles model suffixes, split text, compact capacities and multiword colors", () => {
    expect(deviceLabelSearch("i Phone 14 Pro Max\n256GB\nSpace Black")).toBe("iphone 14 pro max 256 space black")
    expect(deviceLabelSearch("iPhone 15 Pro 1TB Natural Titanium")).toBe("iphone 15 pro 1024 natural titanium")
    expect(deviceLabelSearch("IPHONE 13 MINI 128G BLUE")).toBe("iphone 13 mini 128 blue")
  })
  it("omits missing or ambiguous attributes and rejects non-product text", () => {
    expect(deviceLabelSearch("iPhone 15 128 GB 256 GB Yellow Blue")).toBe("iphone 15")
    expect(deviceLabelSearch("iPhone 15 / iPhone 14 128 GB Yellow")).toBe("")
    expect(deviceLabelSearch("Q7M2K9V4XP 490154203237518 Yellow 128GB")).toBe("")
    expect(deviceLabelSearch("iPhone 15")).toBe("iphone 15")
  })
})
