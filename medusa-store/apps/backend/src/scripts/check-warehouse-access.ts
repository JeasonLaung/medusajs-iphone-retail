import assert from "node:assert/strict"
import type { ExecArgs } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"
import { WAREHOUSE_ROLE, warehouseOrders } from "../lib/warehouse-access"

// Creates a disposable user, makes read-only API requests, then removes the user.
export default async function checkWarehouseAccess({ container }: ExecArgs) {
  const users = container.resolve(Modules.USER)
  const link = container.resolve(ContainerRegistrationKeys.LINK)
  const locations = await container.resolve(Modules.STOCK_LOCATION).listStockLocations({}, { take: null })
  assert(locations.length > 1, "Requires two existing warehouses")
  const allOrders = await warehouseOrders(container, { warehouse: true, location_ids: null })
  const location = locations.find((entry) => allOrders.some((order) => order.fulfillments?.some((f) => f.location_id === entry.id))) || locations[0]
  const user = await users.createUsers({ email: `warehouse-check-${Date.now()}@example.invalid`, metadata: { device_role: "warehouse", warehouse_scope: "bound", warehouse_location_ids: [location.id] } })
  const binding = { [Modules.USER]: { user_id: user.id }, [Modules.RBAC]: { rbac_role_id: WAREHOUSE_ROLE } }
  try {
    await link.create(binding)
    const config = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
    const token = generateJwtToken({ actor_id: user.id, actor_type: "user", auth_identity_id: "warehouse_check", app_metadata: { user_id: user.id, roles: [WAREHOUSE_ROLE] } }, { secret: config.projectConfig.http.jwtSecret, expiresIn: "5m" })
    const get = async (path: string, status = 200) => {
      const response = await fetch(`http://127.0.0.1:9001${path}`, { headers: { authorization: `Bearer ${token}` } })
      const body = await response.json()
      assert.equal(response.status, status, `${path}: ${JSON.stringify(body)}`)
      return body
    }
    const access = await get("/admin/warehouse-access/me")
    assert.deepEqual(access.location_ids, [location.id])
    assert(access.permissions.includes("warehouse_intake:read"))
    for (const path of ["/admin/layouts/sidebar/configuration", "/admin/layouts/order.list/configuration", "/admin/layouts/configurations?limit=1&fields=id", "/admin/regions?fields=id,name", "/admin/sales-channels?fields=id,name"]) await get(path)
    const stock = await get("/admin/stock-locations")
    assert.deepEqual(stock.stock_locations.map((entry) => entry.id), [location.id])
    await get(`/admin/stock-locations/${locations.find((entry) => entry.id !== location.id)!.id}`, 403)
    const orders = await get("/admin/orders?limit=100")
    for (const order of orders.orders) {
      const detail = (await get(`/admin/orders/${order.id}`)).order
      assert(detail.fulfillments.every((f) => f.location_id === location.id))
      assert(detail.items.length)
    }
    const foreign = allOrders.find((order) => order.fulfillments?.every((f) => f.location_id !== location.id))
    if (foreign) await get(`/admin/orders/${foreign.id}`, 403)
    for (const category of ["sale", "returned"]) {
      const body = await get(`/admin/devices?category=${category}&limit=100`)
      assert(body.devices.every((entry) => entry.location_id === location.id))
      assert(body.sources.every((entry) => entry.location_id === location.id))
    }
    await get("/admin/rbac/roles", 403)
    await get("/admin/warehouse-access", 403)
    await get("/admin/customers", 403)
    await users.updateUsers({ id: user.id, metadata: { device_role: "warehouse", warehouse_scope: "bound", warehouse_location_ids: [] } })
    assert.equal((await get("/admin/orders")).count, 0)
    assert.equal((await get("/admin/devices?category=sale")).devices.length, 0)
    assert.equal((await get("/admin/stock-locations")).count, 0)
    console.log(`Warehouse API isolation passed; ${orders.orders.length} assigned orders visible without requiring device records`)
  } finally {
    await link.dismiss(binding)
    await users.deleteUsers([user.id])
  }
}
