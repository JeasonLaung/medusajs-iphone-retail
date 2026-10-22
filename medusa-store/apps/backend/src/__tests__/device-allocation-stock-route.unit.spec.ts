import { GET } from "../api/admin/devices/route"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"

jest.mock("../lib/warehouse-access", () => ({ warehouseAccess: jest.fn().mockResolvedValue({ warehouse: false, location_ids: null }), scopeInventoryLevels: (variant) => variant }))
jest.mock("../workflows/device-operations", () => ({ deviceOperationWorkflow: jest.fn() }))
jest.mock("../workflows/device-source", () => ({ createDeviceSourceWorkflow: jest.fn() }))
jest.mock("../api/warehouse-guards", () => ({ authorizeWarehouseCommand: jest.fn() }))
jest.mock("../api/device-access", () => ({ deviceAccess: jest.fn(), authorizeDeviceCommand: jest.fn(), supplierDenied: jest.fn() }))
const { deviceAccess } = jest.requireMock("../api/device-access")

async function loadStock(devices: any[], levels: any[], supplier = false) {
  deviceAccess.mockResolvedValue({ role: supplier ? "supplier" : "admin", source_id: supplier ? "source" : null, order_ids: ["order"] })
  const service = {
    listDeviceAssignments: jest.fn().mockResolvedValue([]),
    listAndCountDevices: jest.fn().mockResolvedValue([[], 0]),
    listDevices: jest.fn().mockImplementation((filters) => Promise.resolve(devices.filter((d) => filters.state.includes(d.state) && d.category === filters.category))),
    listDeviceSources: jest.fn().mockResolvedValue([{ id: "source", location_id: "own", name: "Own", type: "supplier" }]),
    listDeviceSupplies: jest.fn().mockResolvedValue([]),
    listDeviceVariants: jest.fn().mockResolvedValue([]),
  }
  const graph = jest.fn().mockImplementation(({ entity, fields }) => Promise.resolve({ data: entity === "order"
    ? [{ items: [{ id: "item", variant_id: "variant", quantity: 1 }], fulfillments: [{ location_id: "empty" }] }]
    : fields.includes("inventory_items.inventory.location_levels.stocked_quantity")
      ? [{ id: "variant", inventory_items: [{ inventory: { location_levels: levels } }] }]
      : [{ id: "variant", product: { title: "iPhone" } }] }))
  const services = {
    [DEVICES_MODULE]: service,
    [ContainerRegistrationKeys.QUERY]: { graph },
    [Modules.STOCK_LOCATION]: { listStockLocations: jest.fn().mockImplementation(({ id }) => Promise.resolve(id.map((location) => ({ id: location, name: location })))) },
  }
  const req = { scope: { resolve: (key: string) => services[key] }, auth_context: { actor_id: "user" }, query: { order_id: "order", available_variant_id: "variant", q: "missing", offset: 50, lookup_location_id: "empty" } }
  const res = { json: jest.fn() }
  await GET(req as any, res as any)
  return { stock: res.json.mock.calls[0][0].allocation_stock, service, graph }
}

test("shows stocked and zero-stock warehouses without registered devices, independent of search/page", async () => {
  const { stock, graph } = await loadStock([], [{ location_id: "own", stocked_quantity: "3", reserved_quantity: 2 }, { location_id: "zero", stocked_quantity: 0 }, null])
  expect(stock).toEqual([
    { location_id: "own", name: "own", count: 0, registered_count: 0, stocked_quantity: 3 },
    { location_id: "zero", name: "zero", count: 0, registered_count: 0, stocked_quantity: 0 },
    { location_id: "empty", name: "empty", count: 0, registered_count: 0, stocked_quantity: 0 },
  ])
  expect(graph).toHaveBeenCalledWith(expect.objectContaining({ filters: { id: "variant" }, fields: expect.arrayContaining(["inventory_items.inventory.location_levels.stocked_quantity"]) }))
})

test("counts registered devices still in stock separately from selectable devices", async () => {
  const devices = ["pending", "inspection", "available", "reserved", "packed", "shipped", "retired"].map((state) => ({ id: state, location_id: "own", state, category: "sale" }))
  devices.push({ id: "return", location_id: "own", state: "available", category: "returned" })
  const { stock } = await loadStock(devices, [{ location_id: "own", stocked_quantity: 7 }])
  expect(stock[0]).toMatchObject({ location_id: "own", count: 1, registered_count: 4, stocked_quantity: 7 })
})

test("supplier totals only expose their own warehouse inventory", async () => {
  const { stock, service } = await loadStock([], [{ location_id: "own", stocked_quantity: 3 }, { location_id: "private", stocked_quantity: 12 }], true)
  expect(stock).toEqual([{ location_id: "own", name: "own", count: 0, registered_count: 0, stocked_quantity: 3 }])
  expect(service.listDevices).toHaveBeenCalledWith(expect.objectContaining({ source_id: "source" }), expect.anything())
})
