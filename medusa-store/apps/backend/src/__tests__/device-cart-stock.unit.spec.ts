import { deviceStockCaps } from "../lib/device-stock"
import { validateDeviceCartStock } from "../workflows/hooks/device-cart-stock"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import type { MedusaContainer } from "@medusajs/framework/types"

jest.mock("@medusajs/medusa/core-flows", () => ({
  addToCartWorkflow: { hooks: { validate: jest.fn() } },
  completeCartWorkflow: { hooks: { validate: jest.fn() } },
}))

const device = { variant_id: "v1", location_id: "own", state: "available", category: "sale" }
const sources = [{ id: "self", location_id: "own", type: "self" }, { id: "supplier", location_id: "supplier-location", type: "supplier" }]

test("saleable stock excludes pending, returned, packed and shipped devices", () => {
  const caps = deviceStockCaps([
    device, { ...device, state: "reserved" },
    ...["pending", "inspection", "packed", "shipped", "retired"].map(state => ({ ...device, state })),
    { ...device, category: "returned" },
  ], [], sources)
  expect(caps.get("v1:own")).toBe(2)
  expect(caps.has("v1:legacy-demo")).toBe(false)
})

test("confirmed supplier supply is orderable without registered devices and never double counts them", () => {
  const supply = [{ variant_id: "v1", source_id: "supplier", quantity: 3 }]
  expect(deviceStockCaps([], supply, sources).get("v1:supplier-location")).toBe(3)
  expect(deviceStockCaps([{ ...device, location_id: "supplier-location" }], supply, sources).get("v1:supplier-location")).toBe(3)
})

function setup({ devices = [] as typeof device[], supply = 0, reserved = 0, stocked = 1, location = "own", linked = true } = {}) {
  const services = {
    [DEVICES_MODULE]: {
      listDeviceVariants: jest.fn().mockResolvedValue([{ variant_id: "v1", inventory_item_id: "inv1" }]),
      listDevices: jest.fn().mockResolvedValue(devices),
      listDeviceSupplies: jest.fn().mockResolvedValue(supply ? [{ variant_id: "v1", source_id: "supplier", quantity: supply }] : []),
      listDeviceSources: jest.fn().mockResolvedValue(sources),
    },
    [Modules.INVENTORY]: { listInventoryLevels: jest.fn().mockResolvedValue([{ id: "level1", inventory_item_id: "inv1", location_id: location, stocked_quantity: stocked, reserved_quantity: reserved }]) },
    [ContainerRegistrationKeys.QUERY]: { graph: jest.fn().mockResolvedValue({ data: [{ id: location, sales_channels: linked ? [{ id: "channel" }] : [] }] }) },
  }
  return { resolve: (key: string) => services[key] } as MedusaContainer
}

test("native stock is independent of registered devices and supplier supply", async () => {
  await expect(validateDeviceCartStock(setup(), "channel", [{ variant_id: "v1", quantity: 1 }])).resolves.toBeUndefined()
})

test("an available own device authorizes an order", async () => {
  await expect(validateDeviceCartStock(setup({ devices: [device] }), "channel", [{ variant_id: "v1", quantity: 1 }])).resolves.toBeUndefined()
})

test("confirmed supplier supply authorizes an order without a device record", async () => {
  await expect(validateDeviceCartStock(setup({ supply: 3, stocked: 3, location: "supplier-location" }), "channel", [{ variant_id: "v1", quantity: 3 }])).resolves.toBeUndefined()
})

test("device tracking leaves reservations and sales channel validation to core inventory", async () => {
  for (const settings of [{ reserved: 3 }, { stocked: 0 }, { linked: false }]) {
    await expect(validateDeviceCartStock(setup({ supply: 3, stocked: 3, location: "supplier-location", ...settings }), "channel", [{ variant_id: "v1", quantity: 1 }])).resolves.toBeUndefined()
  }
})

test("device counts do not restrict multiple lines of the same variant", async () => {
  await expect(validateDeviceCartStock(setup({ devices: [device] }), "channel", [{ variant_id: "v1", quantity: 1 }, { variant_id: "v1", quantity: 1 }])).resolves.toBeUndefined()
})


test("does not register additional add-to-cart or checkout inventory caps", () => {
  const { addToCartWorkflow, completeCartWorkflow } = jest.requireMock("@medusajs/medusa/core-flows")
  expect(addToCartWorkflow.hooks.validate).not.toHaveBeenCalled()
  expect(completeCartWorkflow.hooks.validate).not.toHaveBeenCalled()
})
