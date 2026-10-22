import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import { hasPermission } from "@medusajs/framework"
import { assertWarehouseLocation, scopeInventoryLevels, scopeWarehouseOrder, warehouseAccess, warehouseOrders, WAREHOUSE_ROLE } from "../lib/warehouse-access"
import { authorizeWarehouseCommand, guardWarehouseAdmin, scopeWarehouseOrderRequest } from "../api/warehouse-guards"

jest.mock("@medusajs/framework", () => ({ hasPermission: jest.fn().mockResolvedValue(true) }))
const bound = { warehouse: true, location_ids: ["own"] }
const own = { id: "parcel", location_id: "own", canceled_at: null, items: [{ line_item_id: "line", quantity: 1 }] }
const foreign = { id: "other", location_id: "other", items: [{ line_item_id: "foreign", quantity: 1 }] }
function setup(metadata: any = { device_role: "warehouse", warehouse_scope: "bound", warehouse_location_ids: ["own"] }, roles = [WAREHOUSE_ROLE]) {
  const devices = {
    retrieveDevice: jest.fn().mockImplementation(async (id) => ({ id, location_id: id === "foreign" ? "other" : "own" })),
    retrieveDeviceSource: jest.fn().mockImplementation(async (id) => ({ id, type: "self", location_id: id === "foreign" ? "other" : "own" })),
    retrieveDeviceAssignment: jest.fn().mockImplementation(async (id) => ({ id, device_id: id, order_id: "order", fulfillment_id: "parcel" })),
  }
  const fulfillments = { listFulfillments: jest.fn().mockResolvedValue([own]), retrieveFulfillment: jest.fn().mockImplementation(async (id) => id === "other" ? foreign : own) }
  const graph = jest.fn().mockImplementation(async ({ entity }) => ({ data: entity === "user" ? [{ id: "actor", rbac_roles: roles.map((id) => ({ id })) }] : entity === "order_fulfillment" ? [{ order_id: "order", fulfillment_id: own.id }] : [{ id: "order", fulfillments: [own, foreign] }] }))
  const services = { [DEVICES_MODULE]: devices, [Modules.USER]: { listUsers: jest.fn().mockResolvedValue([{ id: "actor", metadata }]) }, [Modules.FULFILLMENT]: fulfillments, [ContainerRegistrationKeys.QUERY]: { graph } }
  const container: any = { resolve: (key) => services[key] }
  return { container, devices, fulfillments, graph }
}
afterEach(() => jest.clearAllMocks())

test("warehouse roles without bindings fail closed; empty bindings never grant all warehouses", async () => {
  expect(await warehouseAccess(setup({}).container, "actor")).toEqual({ warehouse: true, location_ids: [] })
  expect(await warehouseAccess(setup({ warehouse_location_ids: [] }).container, "actor")).toEqual({ warehouse: true, location_ids: [] })
  expect(() => assertWarehouseLocation({ warehouse: true, location_ids: [] }, "own")).toThrow("WAREHOUSE_ACCESS_DENIED")
})
test("platform admins and explicitly unrestricted warehouse managers retain full scope", async () => {
  expect(await warehouseAccess(setup({}, ["role_super_admin"]).container, "actor")).toEqual({ warehouse: false, location_ids: null })
  expect(await warehouseAccess(setup({ device_role: "warehouse", warehouse_scope: "all" }).container, "actor")).toEqual({ warehouse: true, location_ids: null })
})
test("split orders expose only this warehouse's parcels, lines and quantities", () => {
  const original = { id: "order", fulfillments: [own, foreign, { ...own, id: "canceled", canceled_at: new Date() }], items: [{ id: "line", quantity: 3, total: 30, detail: { quantity: 3, raw_quantity: { value: "3" } } }, { id: "foreign", quantity: 1, total: 100 }], total: 130, payment_collections: [{ status: "completed", payments: [{ id: "secret" }] }], customer: { id: "customer", orders: [{ id: "secret" }] } }
  const scoped = scopeWarehouseOrder(original, bound)
  expect(scoped.items).toHaveLength(1)
  expect(scoped.items[0]).toMatchObject({ id: "line", quantity: 1, detail: { quantity: 1 } })
  expect(scoped.total).toBe(10)
  expect(scoped.fulfillments.every((parcel) => parcel.location_id === "own")).toBe(true)
  expect(JSON.stringify(scoped)).not.toContain("secret")
  expect(original.items[0].quantity).toBe(3)
})
test("shared SKU definitions retain only authorized inventory levels", () => {
  const variant = { id: "sku", inventory_items: [{ inventory: { location_levels: [{ location_id: "own", stocked_quantity: 3 }, { location_id: "other", stocked_quantity: 99 }] } }] }
  expect(scopeInventoryLevels(variant, bound).inventory_items[0].inventory.location_levels).toEqual([{ location_id: "own", stocked_quantity: 3 }])
})
test("orders are discovered from warehouse fulfillments even without devices", async () => {
  const { container, fulfillments, graph } = setup()
  expect(await warehouseOrders(container, bound)).toEqual([{ id: "order", fulfillments: [own] }])
  expect(fulfillments.listFulfillments).toHaveBeenCalledWith({ location_id: ["own"] }, { take: null, relations: ["items"] })
  expect(graph).toHaveBeenCalledWith(expect.objectContaining({ entity: "order_fulfillment", filters: { fulfillment_id: ["parcel"] } }))
})
test("native order filters intersect caller IDs rather than allowing a filter bypass", async () => {
  const { container } = setup()
  const req: any = { scope: container, auth_context: { actor_id: "actor" }, filterableFields: { id: "foreign-order" }, queryConfig: { fields: ["id", "customer.orders.*"] } }
  const res: any = { json: jest.fn() }
  await scopeWarehouseOrderRequest(req, res)
  expect(req.filterableFields).toEqual({ $and: [{ id: "foreign-order" }, { id: ["order"] }] })
  expect(req.queryConfig.fields).not.toContain("customer.orders.*")
})
test("batch intake validates every source before any workflow mutation", async () => {
  const { container } = setup()
  await expect(authorizeWarehouseCommand(container, "actor", { action: "intake", devices: [{ source_id: "own" }, { source_id: "foreign" }] } as any)).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
})
test.each([
  { action: "update", device_id: "own", source_id: "own", location_id: "other" },
  { action: "update", device_id: "foreign", source_id: "own", location_id: "own" },
  { action: "make_available", device_ids: ["own", "foreign"] },
  { action: "source", name: "forbidden", type: "self" },
  { action: "save_allocation", order_id: "order", allocations: [{ device_id: "own", item_id: "foreign", fulfillment_id: "other" }] },
  { action: "save_allocation", order_id: "order", allocations: [{ device_id: "foreign", item_id: "line", fulfillment_id: "parcel" }] },
  { action: "save_allocation", order_id: "order", allocations: [{ device_id: "own", item_id: "line" }] },
  { action: "receive", assignment_id: "foreign", source_id: "own" },
])("rejects cross-warehouse or administrative command %#", async (body) => {
  await expect(authorizeWarehouseCommand(setup().container, "actor", body as any)).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
})
test("allows binding a local device to its existing local fulfillment", async () => {
  await expect(authorizeWarehouseCommand(setup().container, "actor", { action: "save_allocation", order_id: "order", allocations: [{ device_id: "own", item_id: "line", fulfillment_id: "parcel" }] })).resolves.toBeUndefined()
})
test("revoked role permissions prevent a previously permitted command", async () => {
  ;(hasPermission as jest.Mock).mockResolvedValueOnce(false)
  await expect(authorizeWarehouseCommand(setup().container, "actor", { action: "delete", device_id: "own" })).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
})
test("direct calls to unrelated admin APIs are forbidden", async () => {
  const req: any = { scope: setup().container, auth_context: { actor_id: "actor", actor_type: "user" }, originalUrl: "/admin/warehouse-access", method: "POST" }
  await expect(guardWarehouseAdmin(req, {} as any, jest.fn())).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
})

test("returns-only users can read shared SKU definitions without intake permission", async () => {
  ;(hasPermission as jest.Mock).mockImplementation(async ({ actions }) => actions.resource === "warehouse_return")
  const req: any = { scope: setup().container, auth_context: { actor_id: "actor", actor_type: "user" }, originalUrl: "/admin/devices/variants", method: "GET", query: {} }
  const next = jest.fn()
  await guardWarehouseAdmin(req, {} as any, next)
  expect(next).toHaveBeenCalledTimes(1)
  ;(hasPermission as jest.Mock).mockResolvedValue(true)
})

test("returned intake and edits require return permission instead of intake permission", async () => {
  const { container, devices } = setup()
  ;(hasPermission as jest.Mock).mockImplementation(async ({ actions }) => actions.resource === "warehouse_return")
  await expect(authorizeWarehouseCommand(container, "actor", { action: "intake", devices: [{ source_id: "own", category: "returned" }] } as any)).resolves.toBeUndefined()
  await expect(authorizeWarehouseCommand(container, "actor", { action: "intake", devices: [{ source_id: "own", category: "sale" }] } as any)).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
  devices.retrieveDevice.mockResolvedValue({ id: "own", location_id: "own", category: "returned" })
  await expect(authorizeWarehouseCommand(container, "actor", { action: "delete", device_id: "own" })).resolves.toBeUndefined()
  await expect(authorizeWarehouseCommand(container, "actor", { action: "update", device_id: "own", location_id: "own", source_id: "own", category: "sale" } as any)).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
  ;(hasPermission as jest.Mock).mockResolvedValue(true)
})

test("warehouse users cannot intake recycled devices even with intake permission", async () => {
  await expect(authorizeWarehouseCommand(setup().container, "actor", { action: "intake", devices: [{ source_id: "own", category: "recycled" }] } as any)).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
})

test.each([
  "/admin/products/product/variants",
  "/admin/product-variants/variant/inventory-items",
])("shared definitions do not open unrestricted linked endpoints: %s", async (path) => {
  const req: any = { scope: setup().container, auth_context: { actor_id: "actor", actor_type: "user" }, originalUrl: path, method: "GET", query: {} }
  await expect(guardWarehouseAdmin(req, {} as any, jest.fn())).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
})
test("shared product requests cannot traverse into another warehouse via linked relations", async () => {
  const req: any = { scope: setup().container, auth_context: { actor_id: "actor", actor_type: "user" }, originalUrl: "/admin/products", method: "GET", query: { fields: "variants.inventory_items.inventory.location_levels.stock_locations.inventory_levels.*" } }
  await expect(guardWarehouseAdmin(req, {} as any, jest.fn())).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
})
