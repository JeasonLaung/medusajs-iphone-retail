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
  const query = { graph: jest.fn().mockImplementation(({ entity, filters }) => Promise.resolve({ data: entity === "stock_location" ? [{ id: filters.id }] : [{ id: filters.id, manage_inventory: true, allow_backorder: false, inventory_items: [{ inventory_item_id: `inv_${filters.id}`, required_quantity: 1 }], product: { title: "iPhone" } }] })) }
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
  it("moves saleable stock when SKU and warehouse change", async () => {
    const { container } = setup()
    const { output } = await prepare({ ...command, actor_id: "user", variant_id: "variant_2", location_id: "loc_2" }, { container })
    expect(output.adjustments).toEqual([
      { inventory_item_id: "inv_variant_1", location_id: "loc_1", adjustment: -1 },
      { inventory_item_id: "inv_variant_2", location_id: "loc_2", adjustment: 1 },
    ])
  })
  it("removes stock on retirement and rejects conflicting device identities", async () => {
    const { container } = setup()
    const { output } = await prepare({ ...command, actor_id: "user", state: "retired" }, { container })
    expect(output.adjustments).toEqual([{ inventory_item_id: "inv_variant_1", location_id: "loc_1", adjustment: -1 }])
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
