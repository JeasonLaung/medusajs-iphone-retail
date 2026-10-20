import DevicesModuleService from "../modules/devices/service"

jest.mock("@medusajs/framework/utils", () => ({
  ...jest.requireActual("@medusajs/framework/utils"),
  MedusaService: () => class {
    softDeleteDevices = jest.fn().mockResolvedValue({})
  },
}))

describe("device deletion service protection", () => {
  it.each([{ ids: "dev_1" }, { ids: ["dev_1", "dev_2"] }])("routes legacy single and bulk deletes through soft deletion", async ({ ids }) => {
    const service = new DevicesModuleService({})
    const context = {}
    await service.deleteDevices(ids, context)
    expect(service.softDeleteDevices).toHaveBeenCalledWith(ids, {}, context)
  })
})
