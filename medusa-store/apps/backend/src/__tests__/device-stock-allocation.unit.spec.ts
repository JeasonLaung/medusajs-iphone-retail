import { createStep } from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import "../workflows/device-operations"

jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn(() => ({})), createWorkflow: jest.fn(() => ({})),
  StepResponse: class { constructor(public output: unknown) {} }, WorkflowResponse: jest.fn(),
}))
jest.mock("@medusajs/medusa/core-flows", () => ({}))
jest.mock("../api/device-access", () => ({ authorizeDeviceCommand: jest.fn() }))

const prepare = (createStep as jest.Mock).mock.calls.find(([name]) => name === "prepare-devices")[1]
const device = { id: "d1", serial_number: "SERIAL", imei: "123456789012345", imei2: null, eid: null, variant_id: "v1", source_id: "s1", location_id: "l1", state: "available", category: "sale" }
const command = { action: "order_assign", serial_number: "SERIAL", order_id: "o1", item_id: "i1", actor_id: "u1" }
function setup(matches: unknown[] = [device], overrides = {}) {
  const service = {
    listDevices: jest.fn().mockResolvedValue(matches),
    retrieveDevice: jest.fn().mockResolvedValue({ ...device, ...overrides }),
    listDeviceAssignments: jest.fn().mockResolvedValue([]),
    retrieveDeviceSource: jest.fn().mockResolvedValue({ id: "s1", location_id: "l1", ...("source_id" in overrides ? { id: overrides.source_id } : {}) }),
    listDeviceSources: jest.fn().mockResolvedValue([{ id: "s1", location_id: "l1" }]),
  }
  const query = { graph: jest.fn(async ({ entity }) => ({ data: entity === "order"
    ? [{ id: "o1", status: "pending", sales_channel_id: "sc1", items: [{ id: "i1", variant_id: "v1", detail: { quantity: 1, fulfilled_quantity: 0 } }] }]
    : entity === "stock_location" ? [{ sales_channels: [{ id: "sc1" }] }]
    : [{ id: "v1", manage_inventory: true, allow_backorder: false, inventory_items: [{ inventory_item_id: "inv1", required_quantity: 1 }], product: { title: "iPhone" } }] })) }
  return { resolve: (key: string) => ({ [DEVICES_MODULE]: service, [Modules.INVENTORY]: {}, [ContainerRegistrationKeys.QUERY]: query })[key] }
}

describe("only registered devices can be allocated", () => {
  it("rejects unknown identifiers before creating a device or source", async () => {
    await expect(prepare(command, { container: setup([]) })).rejects.toThrow("DEVICE_INTAKE_REQUIRED")
  })
  it("uses the existing record and leaves inventory quantities unchanged", async () => {
    const { output } = await prepare(command, { container: setup() })
    expect(output.adjustments).toEqual([])
    expect(output.levels).toEqual([])
    const devices = output.mutations.filter((entry) => entry.table === "Devices")
    expect(devices).toHaveLength(1)
    expect(devices[0]).toMatchObject({ before: device, after: { ...device, state: "reserved" } })
    expect(output.mutations.some((entry) => entry.table === "DeviceSources")).toBe(false)
  })
  it("uses the device source when multiple sources share a warehouse", async () => {
    const { output } = await prepare(command, { container: setup([device], { source_id: "device-supplier" }) })
    const assignment = output.mutations.find((entry) => entry.table === "DeviceAssignments")
    expect(assignment.after.source_id).toBe("device-supplier")
  })
  it("rejects adding identity fields through the old allocation endpoint", async () => {
    await expect(prepare({ ...command, imei2: "223456789012345" }, { container: setup() })).rejects.toThrow("不一致")
  })
  it("rejects conflicting identifiers belonging to different devices", async () => {
    await expect(prepare(command, { container: setup([device, { ...device, id: "d2" }]) })).rejects.toThrow("多个设备")
  })
  it("blocks direct supplier registration during allocation", async () => {
    await expect(prepare({ action: "supplier_assign", order_id: "o1", item_id: "i1", actor_id: "u1", device }, { container: setup() })).rejects.toThrow("DEVICE_INTAKE_REQUIRED")
  })
  it.each([{ state: "inspection" }, { state: "reserved" }, { state: "shipped" }, { category: "returned" }, { variant_id: "v2" }])("rejects ineligible registered devices: %j", async (overrides) => {
    await expect(prepare({ action: "assign", order_id: "o1", item_id: "i1", device_id: "d1", actor_id: "u1" }, { container: setup([device], overrides) })).rejects.toThrow()
  })
})
