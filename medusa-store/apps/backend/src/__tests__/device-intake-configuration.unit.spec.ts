import { IntakeDevice } from "../api/admin/devices/validators"
import { assertVariantConfiguration, intakeDeviceConfiguration } from "../modules/devices/policy"

const variant = {
  options: [
    { value: "S", option: { title: "Condition" } },
    { value: "100%", option: { title: "Battery" } },
  ],
}

describe("device intake configuration", () => {
  it("records grade and battery from the SKU when the form only sends identity and costs", () => {
    const intake = IntakeDevice.parse({ serial_number: "TEST123", variant_id: "variant", source_id: "source" })
    const configuration = intakeDeviceConfiguration(variant, intake)
    expect(configuration).toEqual({ grade: "S", battery_health: 100 })
    expect(() => assertVariantConfiguration(variant, configuration)).not.toThrow()
  })

  it("does not invent grade or battery for a legacy SKU without those options", () => {
    expect(intakeDeviceConfiguration({ options: [{ value: "128 GB", option: { title: "Storage" } }] }, {}))
      .toEqual({ grade: null, battery_health: null })
  })

  it("preserves explicit CSV measurements and rejects conflicting configurations", () => {
    const sku = { options: [variant.options[0], { value: "85%", option: { title: "Battery" } }] }
    const measured = intakeDeviceConfiguration(sku, { battery_health: 92 })
    expect(measured).toEqual({ grade: "S", battery_health: 92 })
    expect(() => assertVariantConfiguration(sku, measured)).not.toThrow()
    expect(() => assertVariantConfiguration(sku, intakeDeviceConfiguration(sku, { grade: "A" }))).toThrow()
    expect(() => assertVariantConfiguration(sku, intakeDeviceConfiguration(sku, { battery_health: 84 }))).toThrow()
  })
})
