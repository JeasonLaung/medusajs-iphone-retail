import { createStep } from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import "../workflows/device-operations"

jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn(() => ({})), createWorkflow: jest.fn(() => ({})),
  StepResponse: class { constructor(public output: unknown) {} }, WorkflowResponse: jest.fn(),
}))
jest.mock("@medusajs/medusa/core-flows", () => ({}))
jest.mock("../api/warehouse-guards", () => ({ authorizeWarehouseCommand: jest.fn() }))
jest.mock("../api/device-access", () => ({ authorizeDeviceCommand: jest.fn() }))

const prepare = (createStep as jest.Mock).mock.calls.find(([name]) => name === "prepare-devices")[1]
const device = { id: "d1", serial_number: "SERIAL", imei: "123456789012345", imei2: null, eid: null, variant_id: "v1", source_id: "s1", location_id: "l1", state: "available", category: "sale" }
const command = { action: "order_assign", serial_number: "SERIAL", order_id: "o1", item_id: "i1", actor_id: "u1" }
function setup(matches: unknown[] = [device], overrides = {}, paymentCollections = [{ status: "completed" }]) {
  const service = {
    listDevices: jest.fn().mockResolvedValue(matches),
    retrieveDevice: jest.fn().mockResolvedValue({ ...device, ...overrides }),
    listDeviceAssignments: jest.fn().mockResolvedValue([]),
    retrieveDeviceSource: jest.fn().mockResolvedValue({ id: "s1", location_id: "l1", ...("source_id" in overrides ? { id: overrides.source_id } : {}) }),
    listDeviceSources: jest.fn().mockResolvedValue([{ id: "s1", location_id: "l1" }]),
  }
  const query = { graph: jest.fn(async ({ entity }) => ({ data: entity === "order"
    ? [{ id: "o1", status: "pending", payment_collections: paymentCollections, sales_channel_id: "sc1", items: [{ id: "i1", variant_id: "v1", detail: { quantity: 1, fulfilled_quantity: 0 } }] }]
    : entity === "stock_location" ? [{ sales_channels: [{ id: "sc1" }] }]
    : [{ id: "v1", manage_inventory: true, allow_backorder: false, inventory_items: [{ inventory_item_id: "inv1", required_quantity: 1 }], product: { title: "iPhone" } }] })) }
  return { resolve: (key: string) => ({ [DEVICES_MODULE]: service, [Modules.INVENTORY]: {}, [ContainerRegistrationKeys.QUERY]: query })[key] }
}

describe("only registered devices can be allocated", () => {
  it.each([[], [{ status: "not_paid" }], [{ status: "authorized" }], [{ status: "partially_captured" }], [{ status: "completed" }, { status: "pending" }]])("rejects allocation before full payment: %j", async (...collections) => {
    await expect(prepare(command, { container: setup([device], {}, collections) })).rejects.toThrow("DEVICE_PAYMENT_REQUIRED")
  })
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

function fulfillmentSetup({ quantity = 3, fulfilled = 0, location = "l1", reserved = 3, previous = false, native = false } = {}) {
  const storedDevice = { ...device, location_id: location, state: previous ? "reserved" : "available" }
  const assignment = { id: "a1", device_id: "d1", item_id: "i1", order_id: "o1", source_id: "s1", status: "reserved" }
  const service = {
    retrieveDevice: jest.fn().mockResolvedValue(storedDevice),
    listDevices: jest.fn().mockResolvedValue([storedDevice]),
    listDeviceAssignments: jest.fn(async (filter) => filter.order_id && previous ? [assignment] : []),
    retrieveDeviceAssignment: jest.fn().mockResolvedValue(assignment),
    retrieveDeviceSource: jest.fn().mockResolvedValue({ id: "s1", location_id: location }),
    listDeviceVariants: jest.fn().mockResolvedValue([{ variant_id: "v1", inventory_item_id: "inv1" }]),
  }
  const inventory = {
    listReservationItems: jest.fn().mockResolvedValue([{ id: "r1", inventory_item_id: "inv1", location_id: "l1", quantity: reserved }]),
    listInventoryLevels: jest.fn().mockResolvedValue([{ stocked_quantity: 10, reserved_quantity: reserved }]),
  }
  const fulfillment = { retrieveFulfillment: jest.fn().mockResolvedValue({ id: "f1", location_id: "l1", metadata: {}, items: [{ line_item_id: "i1", quantity: 1 }] }) }
  const query = { graph: jest.fn(async ({ entity }) => ({ data: entity === "order"
    ? [{ id: "o1", status: "pending", payment_collections: [{ status: "completed" }], sales_channel_id: "sc1", items: [{ id: "i1", variant_id: "v1", detail: { quantity, fulfilled_quantity: fulfilled } }], fulfillments: native ? [{ id: "f1", location_id: "l1", items: [{ line_item_id: "i1", quantity: 1 }] }] : [] }]
    : entity === "stock_location" ? [{ sales_channels: [{ id: "sc1" }] }]
    : [{ id: "v1", manage_inventory: true, allow_backorder: false, inventory_items: [{ inventory_item_id: "inv1", required_quantity: 1 }], product: { title: "iPhone" } }] })) }
  return { resolve: (key: string) => ({ [DEVICES_MODULE]: service, [Modules.INVENTORY]: inventory, [Modules.FULFILLMENT]: fulfillment, [ContainerRegistrationKeys.QUERY]: query })[key] }
}
const createCommand = { action: "create_fulfillment", order_id: "o1", actor_id: "u1", allocations: [{ item_id: "i1", device_id: "d1" }], fulfillment: { location_id: "l1", shipping_option_id: "so1", no_notification: true, items: [{ id: "i1", quantity: 2 }] } }
describe("devices within native fulfillment", () => {
  it("creates a fulfillment before device identities are known", async () => {
    const { output } = await prepare({ ...createCommand, allocations: [] }, { container: fulfillmentSetup() })
    expect(output.fulfillment).toMatchObject({ ...createCommand.fulfillment, order_id: "o1" })
    expect(output.mutations).toEqual([])
    expect(output.adjustments).toEqual([])
  })
  it("reserves the full fulfilled quantity when only some devices are supplied", async () => {
    const { output } = await prepare(createCommand, { container: fulfillmentSetup() })
    expect(output.fulfillment).toMatchObject({ ...createCommand.fulfillment, order_id: "o1" })
    expect(output.reservations).toEqual([{ id: "r1", location_id: "l1", quantity: 2 }])
    expect(output.remainingReservations[0].quantity).toBe(1)
    expect(output.mutations.find((entry) => entry.table === "Devices").after.state).toBe("packed")
  })
  it("rejects a selected device outside the requested items", async () => {
    await expect(prepare({ ...createCommand, fulfillment: { ...createCommand.fulfillment, items: [{ id: "i1", quantity: 0 }] } }, { container: fulfillmentSetup() })).rejects.toThrow("DEVICE_FULFILLMENT_LOCATION")
  })
  it("cannot move an already fulfilled quantity into a new fulfillment", async () => {
    await expect(prepare(createCommand, { container: fulfillmentSetup({ quantity: 1, fulfilled: 1, native: true }) })).rejects.toThrow("已配足或已履约")
  })
  it("rejects devices from another warehouse", async () => {
    await expect(prepare(createCommand, { container: fulfillmentSetup({ location: "l2" }) })).rejects.toThrow("DEVICE_FULFILLMENT_LOCATION")
  })
  it("keeps a product-row allocation reserved until fulfillment is created", async () => {
    const { output } = await prepare({ action: "save_allocation", order_id: "o1", actor_id: "u1", reserve_only: true, allocations: createCommand.allocations }, { container: fulfillmentSetup() })
    expect(output.fulfillment).toBeUndefined()
    expect(output.reservations).toEqual([])
    expect(output.mutations.find((entry) => entry.table === "Devices").after.state).toBe("reserved")
  })
  it("includes a previously reserved device when creating fulfillment", async () => {
    const { output } = await prepare({ ...createCommand, allocations: [{ ...createCommand.allocations[0], assignment_id: "a1" }] }, { container: fulfillmentSetup({ previous: true }) })
    expect(output.mutations.find((entry) => entry.table === "DeviceAssignments").after.status).toBe("packed")
  })
  it("adds devices to an existing same-warehouse fulfillment without touching inventory", async () => {
    const { output } = await prepare({ action: "save_allocation", order_id: "o1", actor_id: "u1", allocations: [{ ...createCommand.allocations[0], fulfillment_id: "f1" }] }, { container: fulfillmentSetup({ quantity: 1, fulfilled: 1, native: true }) })
    expect(output.fulfillment).toBeUndefined()
    expect(output.adjustments).toEqual([])
    expect(output.reservations).toEqual([])
    expect(output.mutations.find((entry) => entry.table === "DeviceAssignments").after).toMatchObject({ status: "packed", fulfillment_id: "f1" })
  })
})
