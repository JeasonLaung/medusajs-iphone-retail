import { createStep } from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { DeviceCommand } from "../api/admin/devices/validators"
import { DEVICES_MODULE } from "../modules/devices"
import "../workflows/device-operations"

jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn(() => ({})), createWorkflow: jest.fn(() => ({})),
  StepResponse: class { constructor(public output: unknown) {} }, WorkflowResponse: jest.fn(),
}))
jest.mock("@medusajs/medusa/core-flows", () => ({}))
jest.mock("../api/warehouse-guards", () => ({ authorizeWarehouseCommand: jest.fn() }))
jest.mock("../api/device-access", () => ({ authorizeDeviceCommand: jest.fn() }))

const device = { id: "dev_1", serial_number: "FIXED", variant_id: "variant_1", source_id: "source_1", location_id: "loc_1", imei: "123456789012345", imei2: null, eid: null, category: "sale", state: "available", grade: "A", battery_health: 90, cost_cents: 12345, currency_code: "eur", bin: "A1", notes: "", photos: [] }
const command = { ...device, action: "update", device_id: device.id }
delete (command as Partial<typeof command>).id
delete (command as Partial<typeof command>).serial_number
const prepare = (createStep as jest.Mock).mock.calls.find(([name]) => name === "prepare-devices")[1]

function setup(assignments: unknown[] = [], duplicates = [device]) {
  const service = {
    retrieveDevice: jest.fn().mockResolvedValue(device),
    retrieveDeviceSource: jest.fn().mockImplementation((id) => Promise.resolve({ id, type: "self", location_id: id === "source_1" ? "loc_1" : "loc_2" })),
    listDeviceAssignments: jest.fn().mockResolvedValue(assignments),
    listDeviceVariants: jest.fn().mockImplementation(({ variant_id }) => Promise.resolve([{ inventory_item_id: `inv_${variant_id}` }])),
    listDevices: jest.fn().mockResolvedValue(duplicates),
  }
  const inventory = { listInventoryLevels: jest.fn().mockResolvedValue([{ stocked_quantity: 5, reserved_quantity: 1 }]) }
  const query = { graph: jest.fn().mockImplementation(({ entity, filters = { id: "variant_1" } }) => Promise.resolve({ data: entity === "stock_location" ? [{ id: filters.id }] : [{ id: filters.id, manage_inventory: true, allow_backorder: false, inventory_items: [{ inventory_item_id: `inv_${filters.id}`, required_quantity: 1 }], product: { title: "iPhone" } }] })) }
  const container = { resolve: (key: string) => ({ [DEVICES_MODULE]: service, [Modules.INVENTORY]: inventory, [ContainerRegistrationKeys.QUERY]: query })[key] }
  return { container, service, inventory }
}

describe("device editing", () => {
  it("rejects serial and internal identity injection, invalid radio values and invalid measurements", () => {
    expect(DeviceCommand.safeParse(command).success).toBe(true)
    for (const extra of [{ serial_number: "OTHER" }, { id: "other" }, { actor_id: "user" }, { battery_health: 101 }, { cost_cents: -1 }, { imei: "123" }, { eid: "123" }])
      expect(DeviceCommand.safeParse({ ...command, ...extra }).success).toBe(false)
  })
  it("edits information without changing the serial or adding stock", async () => {
    const { container } = setup()
    const { output } = await prepare({ ...command, actor_id: "user", serial_number: "OTHER", notes: "updated" }, { container })
    expect(output.adjustments).toEqual([])
    expect(output.mutations[0].after).toMatchObject({ serial_number: "FIXED", notes: "updated" })
    expect(output.mutations[1].after).toMatchObject({ action: "update", device_id: device.id })
  })
  it("keeps stock unchanged when SKU and warehouse change", async () => {
    const { container } = setup()
    const { output } = await prepare({ ...command, actor_id: "user", variant_id: "variant_2", location_id: "loc_2" }, { container })
    expect(output.adjustments).toEqual([])
  })
  it("preserves stock on retirement and rejects conflicting device identities", async () => {
    const { container } = setup()
    const { output } = await prepare({ ...command, actor_id: "user", state: "retired" }, { container })
    expect(output.adjustments).toEqual([])
    await expect(prepare({ ...command, actor_id: "user" }, { container: setup([], [{ ...device, id: "dev_other" }]).container })).rejects.toThrow()
  })
  it("protects allocation fields while allowing notes on allocated devices", async () => {
    const { container, service } = setup([{}])
    service.retrieveDevice.mockResolvedValue({ ...device, state: "reserved" })
    await expect(prepare({ ...command, actor_id: "user", state: "reserved", variant_id: "variant_2" }, { container })).rejects.toThrow("DEVICE_EDIT_ALLOCATED")
    const { output } = await prepare({ ...command, actor_id: "user", state: "reserved", notes: "corrected" }, { container })
    expect(output.adjustments).toEqual([])
    expect(output.mutations[0].after.notes).toBe("corrected")
  })
  it("rejects creating allocation states outside the order workflow", async () => {
    await expect(prepare({ ...command, actor_id: "user", state: "packed" }, { container: setup().container })).rejects.toThrow("DEVICE_EDIT_STATE")
  })
})


describe("moving sale devices to returned devices", () => {
  const transfer = { action: "transfer_return", actor_id: "user", device_id: device.id, notes: "Screen requires a new inspection" }
  function ownWarehouse(assignments: unknown[] = []) {
    const base = setup(assignments)
    Object.assign(base.service, { listDeviceSources: jest.fn().mockResolvedValue([{ id: "source_1", type: "self", location_id: "loc_1" }]) })
    return base
  }
  it("validates the transfer reason and rejects injected device fields", () => {
    const { actor_id: _actor, ...body } = transfer
    expect(DeviceCommand.safeParse(body).success).toBe(true)
    for (const extra of [{ notes: " " }, { category: "sale" }, { state: "available" }]) expect(DeviceCommand.safeParse({ ...body, ...extra }).success).toBe(false)
  })
  it.each(["available", "pending", "inspection"])("moves an unallocated %s device to returned inspection and logs the change", async (state) => {
    const { container, service } = ownWarehouse()
    service.retrieveDevice.mockResolvedValue({ ...device, state })
    const { output } = await prepare(transfer, { container })
    expect(output.mutations.find((m: any) => m.table === "Devices").after).toEqual({ ...device, category: "returned", state: "inspection" })
    expect(output.mutations.find((m: any) => m.table === "DeviceEvents").after).toMatchObject({ action: "transfer_return", actor_id: "user", device_id: device.id, data: { notes: transfer.notes, before: { category: "sale", state }, after: { category: "returned", state: "inspection" } } })
    expect(output.adjustments).toEqual([])
    expect(output.newReservations).toEqual([])
  })
  it.each(["reserved", "packed", "shipped", "repair_ready", "retired", "supplier_returned"])("rejects %s devices", async (state) => {
    const { container, service } = ownWarehouse()
    service.retrieveDevice.mockResolvedValue({ ...device, state })
    await expect(prepare(transfer, { container })).rejects.toThrow("DEVICE_TRANSFER_RETURN_INVALID")
  })
  it("rejects active assignments, returned/recycled devices and devices outside an owned warehouse", async () => {
    await expect(prepare(transfer, { container: ownWarehouse([{}]).container })).rejects.toThrow("DEVICE_TRANSFER_RETURN_INVALID")
    for (const category of ["returned", "recycled"]) {
      const { container, service } = ownWarehouse()
      service.retrieveDevice.mockResolvedValue({ ...device, category })
      await expect(prepare(transfer, { container })).rejects.toThrow("DEVICE_TRANSFER_RETURN_INVALID")
    }
    const { container, service } = ownWarehouse()
    Object.assign(service, { listDeviceSources: jest.fn().mockResolvedValue([]) })
    await expect(prepare(transfer, { container })).rejects.toThrow("DEVICE_TRANSFER_RETURN_INVALID")
  })
})

describe("returned device processing", () => {
  const inspect = { action: "inspect", actor_id: "user", device_id: device.id, result: "pass", grade: "A", battery_health: 90, notes: "Inspection completed" }
  function returned(repair = false) {
    const base = setup(repair ? [{ after_sales_request_id: "request_1" }] : [])
    base.service.retrieveDevice.mockResolvedValue({ ...device, category: "returned", state: "inspection" })
    Object.assign(base.service, { listDeviceSources: jest.fn().mockResolvedValue([{ id: "source_1", type: "self" }]) })
    const container = { resolve: (key: string) => base.container.resolve(key) || { retrieveAfterSalesRequest: jest.fn().mockResolvedValue({ type: "repair", status: "processing" }) } }
    return { ...base, container }
  }
  it("moves passing returns to sale devices and writes the operator, transition and notes without changing stock", async () => {
    const { output } = await prepare(inspect, { container: returned().container })
    expect(output.mutations.find((m: any) => m.table === "Devices").after).toMatchObject({ state: "available", category: "sale" })
    expect(output.mutations.find((m: any) => m.table === "DeviceEvents").after).toMatchObject({ action: "inspect", actor_id: "user", device_id: device.id, data: { notes: inspect.notes, before: { state: "inspection" }, after: { state: "available" } } })
    expect(output.adjustments).toEqual([])
  })
  it("keeps customer repair devices awaiting return instead of offering them for sale", async () => {
    const { output } = await prepare(inspect, { container: returned(true).container })
    expect(output.mutations.find((m: any) => m.table === "Devices").after).toMatchObject({ state: "repair_ready", category: "returned" })
  })
  it("attaches verified photos to this processing record with the operator while preserving intake photos", async () => {
    const photo = { id: "file_processing", url: "/static/device-processing.jpg" }
    const base = returned()
    const files = { retrieveFile: jest.fn().mockResolvedValue({ ...photo, url: "http://localhost:9001" + photo.url }) }
    const container = { resolve: (key: string) => key === Modules.FILE ? files : base.container.resolve(key) }
    const { output } = await prepare({ ...inspect, photos: [photo] }, { container })
    expect(files.retrieveFile).toHaveBeenCalledWith(photo.id)
    expect(output.mutations.find((m: any) => m.table === "DeviceEvents").after).toMatchObject({ actor_id: "user", data: { photos: [photo], notes: inspect.notes } })
    expect(output.mutations.find((m: any) => m.table === "Devices").after.photos).toEqual(device.photos)
  })
  it("rejects mismatched file URLs before recording photos or changing status", async () => {
    const base = returned()
    const files = { retrieveFile: jest.fn().mockResolvedValue({ id: "file_processing", url: "http://localhost:9001/static/device-real.jpg" }) }
    const container = { resolve: (key: string) => key === Modules.FILE ? files : base.container.resolve(key) }
    await expect(prepare({ ...inspect, photos: [{ id: "file_processing", url: "/static/device-other.jpg" }] }, { container })).rejects.toThrow("DEVICE_PHOTO_INVALID")
  })
  it("validates photo count and prevents clients from supplying the operator", () => {
    const { actor_id: _actor, ...body } = inspect
    const photo = { id: "file_processing", url: "/static/device-processing.jpg" }
    expect(DeviceCommand.safeParse({ ...body, photos: [photo] }).success).toBe(true)
    expect(DeviceCommand.safeParse({ ...body, photos: Array(9).fill(photo) }).success).toBe(false)
    expect(DeviceCommand.safeParse({ ...body, photos: [{ ...photo, url: "javascript:alert(1)" }] }).success).toBe(false)
    expect(DeviceCommand.safeParse({ ...body, actor_id: "another-user" }).success).toBe(false)
  })
  it.each(["", " ", undefined])("rejects state changes without an operator (%s)", async (actor_id) => {
    await expect(prepare({ ...inspect, actor_id }, { container: returned().container })).rejects.toThrow("DEVICE_ACTOR_REQUIRED")
  })
  it.each([["repair", "repair"], ["retire", "retired"]])("records %s while retaining the returned category", async (result, state) => {
    const { output } = await prepare({ ...inspect, result }, { container: returned().container })
    expect(output.mutations.find((m: any) => m.table === "Devices").after).toMatchObject({ state, category: "returned" })
  })
  it("rejects supplier returns for devices without a supplier source and repeated processing of available devices", async () => {
    await expect(prepare({ ...inspect, result: "supplier_return" }, { container: returned().container })).rejects.toThrow()
    const { container, service } = returned()
    service.retrieveDevice.mockResolvedValue({ ...device, category: "returned" })
    await expect(prepare(inspect, { container })).rejects.toThrow()
  })
})

describe("device soft deletion and rollback", () => {
  const writeStep = (createStep as jest.Mock).mock.calls.find(([name]) => name === "write-devices")
  it("soft deletes a device without physically deleting its record", async () => {
    const service = { softDeleteDevices: jest.fn().mockResolvedValue(undefined), deleteDevices: jest.fn() }
    const mutation = { table: "Devices", before: device, after: device, deleted: true }
    await writeStep[1]({ plan: { action: "delete", mutations: [mutation] } }, { container: { resolve: () => service } })
    expect(service.softDeleteDevices).toHaveBeenCalledWith(device.id)
    expect(service.deleteDevices).not.toHaveBeenCalled()
  })
  it("restores a soft-deleted device when a later workflow step fails", async () => {
    const service = { restoreDevices: jest.fn().mockResolvedValue(undefined), deleteDevices: jest.fn() }
    await writeStep[2]([{ table: "Devices", before: device, after: device, deleted: true }], { container: { resolve: () => service } })
    expect(service.restoreDevices).toHaveBeenCalledWith(device.id)
    expect(service.deleteDevices).not.toHaveBeenCalled()
  })
  it("soft deletes newly created devices during intake rollback", async () => {
    const service = { softDeleteDevices: jest.fn().mockResolvedValue(undefined), deleteDevices: jest.fn() }
    await writeStep[2]([{ table: "Devices", after: device }], { container: { resolve: () => service } })
    expect(service.softDeleteDevices).toHaveBeenCalledWith(device.id)
    expect(service.deleteDevices).not.toHaveBeenCalled()
  })
})


describe("independent device intake and bulk availability", () => {
  const pending = { ...device, state: "pending" }
  it("makes ordinary intake immediately available without changing stock", async () => {
    const { container } = setup([], [])
    const { output } = await prepare({ action: "intake", actor_id: "user", devices: [{ ...device, serial_number: "NEW", category: "sale" }] }, { container })
    expect(output.adjustments).toEqual([])
    expect(output.mutations.find((m: any) => m.table === "Devices").after.state).toBe("available")
  })
  it("keeps returned intake awaiting inspection without saleable stock", async () => {
    const { container } = setup([], [])
    const { output } = await prepare({ action: "intake", actor_id: "user", devices: [{ ...device, serial_number: "RETURN", category: "returned" }] }, { container })
    expect(output.adjustments).toEqual([])
    expect(output.mutations.find((m: any) => m.table === "Devices").after.state).toBe("inspection")
  })
  it("publishes legacy pending devices without adding stock and records each transition", async () => {
    const { container, service } = setup()
    service.retrieveDevice.mockImplementation(async (id) => ({ ...pending, id }))
    const { output } = await prepare({ action: "make_available", device_ids: ["dev_1", "dev_2"], actor_id: "user" }, { container })
    expect(output.adjustments).toEqual([])
    expect(output.mutations.filter((m: any) => m.table === "Devices").map((m: any) => m.after.state)).toEqual(["available", "available"])
    expect(output.mutations.filter((m: any) => m.table === "DeviceEvents")).toHaveLength(2)
  })
  it("rejects repeat confirmation, duplicate IDs, supplier devices and mixed invalid batches", async () => {
    for (const invalid of [device, { ...pending, category: "returned" }, { ...pending, state: "inspection" }]) {
      const { container, service } = setup()
      service.retrieveDevice.mockImplementation(async (id) => id === "dev_1" ? pending : invalid)
      await expect(prepare({ action: "make_available", device_ids: ["dev_1", "dev_2"], actor_id: "user" }, { container })).rejects.toThrow("DEVICE_PENDING_REQUIRED")
    }
    const { container, service } = setup()
    service.retrieveDevice.mockResolvedValue(pending)
    service.retrieveDeviceSource.mockResolvedValue({ type: "supplier" })
    await expect(prepare({ action: "make_available", device_ids: ["dev_1"], actor_id: "user" }, { container })).rejects.toThrow("DEVICE_PENDING_REQUIRED")
    expect(DeviceCommand.safeParse({ action: "make_available", device_ids: ["dev_1", "dev_1"] }).success).toBe(false)
    expect(DeviceCommand.safeParse({ action: "make_available", device_ids: [] }).success).toBe(false)
  })
  it("allows publishing legacy pending devices without changing stock", async () => {
    const { container, service } = setup()
    service.retrieveDevice.mockResolvedValue(pending)
    const { output } = await prepare({ ...command, actor_id: "user", state: "available" }, { container })
    expect(output.adjustments).toEqual([])
    expect(output.mutations[0].after.state).toBe("available")
  })
})


describe("mobile intake deletion scope", () => {
  const session = { nonce: "session_nonce", source_id: "source_1", category: "sale" }
  const input = { action: "delete", device_id: device.id, actor_id: "user", mobile_intake_session: session }
  const event = { data: { mobile_intake_nonce: session.nonce } }
  it("soft deletes an available device entered through the same link without changing inventory", async () => {
    const { container, service } = setup()
    service.retrieveDevice.mockResolvedValue(device)
    Object.assign(service, { listDeviceEvents: jest.fn().mockResolvedValue([event]) })
    const { output } = await prepare(input, { container })
    expect(output.adjustments).toEqual([])
    expect(output.mutations[0]).toMatchObject({ table: "Devices", deleted: true })
  })
  it("rejects other links, warehouses, categories and devices already allocated", async () => {
    for (const [target, events] of [
      [{ ...device, state: "pending" }, []],
      [{ ...device, state: "pending" }, [{ data: { mobile_intake_nonce: "other_link" } }]],
      [{ ...device, state: "pending", source_id: "other" }, [event]],
      [{ ...device, state: "pending", category: "recycled" }, [event]],
      [{ ...device, state: "reserved" }, [event]],
    ] as const) {
      const { container, service } = setup()
      service.retrieveDevice.mockResolvedValue(target)
      Object.assign(service, { listDeviceEvents: jest.fn().mockResolvedValue(events) })
      await expect(prepare(input, { container })).rejects.toThrow("MOBILE_INTAKE_DELETE_DENIED")
    }
  })
})


describe("device records never own stock", () => {
  it("records supplier availability without changing native stock", async () => {
    const { container, service, inventory } = setup()
    service.retrieveDeviceSource.mockResolvedValue({ id: "supplier", type: "supplier", location_id: "loc_2" })
    Object.assign(service, { listDeviceSupplies: jest.fn().mockResolvedValue([{ id: "supply", quantity: 5 }]) })
    const { output } = await prepare({ action: "supply", source_id: "supplier", variant_id: "variant_1", quantity: 20, actor_id: "user" }, { container })
    expect(output.adjustments).toEqual([])
    expect(output.updates).toEqual([])
    expect(inventory.listInventoryLevels).not.toHaveBeenCalled()
    expect(output.mutations[0].after.quantity).toBe(20)
  })
  it("enables serial tracking without resetting stock or reservations", async () => {
    const { container, service, inventory } = setup()
    service.listDeviceVariants.mockResolvedValue([])
    const { output } = await prepare({ action: "enroll", variant_id: "variant_1", actor_id: "user" }, { container })
    expect(output.updates).toEqual([])
    expect(output.adjustments).toEqual([])
    expect(inventory.listInventoryLevels).not.toHaveBeenCalled()
    expect(output.mutations[0].table).toBe("DeviceVariants")
  })
  it.each(["delete", "retire"])("%s of an available device preserves stock", async (action) => {
    const { container } = setup()
    const { output } = await prepare({ action, device_id: device.id, actor_id: "user" }, { container })
    expect(output.adjustments).toEqual([])
    expect(output.updates).toEqual([])
    expect(output.levels).toEqual([])
  })
})
