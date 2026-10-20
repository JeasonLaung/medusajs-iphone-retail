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
const assignment = { id: "a1", device_id: "d1", order_id: "o1", item_id: "i1", source_id: "s1", status: "packed", fulfillment_id: "f1" }

function setup(status = "packed", fulfillment = {}, sibling = {}) {
  const selected = { ...assignment, status }
  const other = { ...assignment, id: "a2", device_id: "d2", ...sibling }
  const service = {
    retrieveDeviceAssignment: jest.fn().mockResolvedValue(selected),
    retrieveDevice: jest.fn(async (id) => ({ id, state: status })),
    listDeviceAssignments: jest.fn().mockResolvedValue([selected, other]),
  }
  const query = { graph: jest.fn(async ({ entity }) => ({ data: entity === "order"
    ? [{ id: "o1", status: "pending", items: [] }]
    : [{ id: "f1", shipped_at: null, delivered_at: null, canceled_at: null, ...fulfillment }] })) }
  const container = { resolve: (key: string) => ({ [DEVICES_MODULE]: service, [Modules.INVENTORY]: {}, [ContainerRegistrationKeys.QUERY]: query })[key] }
  const run = () => prepare({ action: "release", assignment_id: "a1", actor_id: "u1" }, { container })
  return { run, service, query }
}

describe("release allocations before shipment", () => {
  it("releases an unfulfilled device without adjusting stock", async () => {
    const { output } = await setup("reserved").run()
    expect(output.cancel).toBeUndefined()
    expect(output.adjustments).toEqual([])
    expect(output.mutations).toEqual(expect.arrayContaining([
      expect.objectContaining({ table: "Devices", after: { id: "d1", state: "available" } }),
      expect.objectContaining({ table: "DeviceAssignments", after: expect.objectContaining({ id: "a1", status: "canceled", fulfillment_id: null }) }),
    ]))
  })
  it("cancels the native fulfillment once and keeps its other devices allocated", async () => {
    const { output } = await setup().run()
    expect(output.cancel).toEqual({ order_id: "o1", fulfillment_id: "f1" })
    expect(output.adjustments).toEqual([])
    expect(output.mutations).toEqual(expect.arrayContaining([
      expect.objectContaining({ table: "Devices", after: { id: "d1", state: "available" } }),
      expect.objectContaining({ table: "Devices", after: { id: "d2", state: "reserved" } }),
      expect.objectContaining({ table: "DeviceAssignments", before: expect.objectContaining({ id: "a2", status: "packed" }), after: expect.objectContaining({ id: "a2", status: "reserved", fulfillment_id: null }) }),
    ]))
  })
  it.each(["shipped", "returned", "canceled"])("rejects %s allocations", async (status) => {
    await expect(setup(status).run()).rejects.toThrow("只有未发货的配货可以解除")
  })
  it.each(["shipped_at", "delivered_at", "canceled_at"])("rejects native fulfillment with %s despite stale assignment status", async (field) => {
    await expect(setup("packed", { [field]: new Date() }).run()).rejects.toThrow("履约不存在或已发货")
  })
  it.each([{ status: "shipped" }, { order_id: "o2" }, { source_id: "s2" }])("rejects conflicting parcel assignments: %j", async (sibling) => {
    await expect(setup("packed", {}, sibling).run()).rejects.toThrow("只有尚未发出的履约可以取消")
  })
  it("rejects a changed device state before writing", async () => {
    const { run, service } = setup()
    service.retrieveDevice.mockResolvedValue({ id: "d1", state: "shipped" })
    await expect(run()).rejects.toThrow("设备状态已改变")
  })
})
