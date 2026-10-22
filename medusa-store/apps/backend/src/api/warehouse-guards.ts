import type { MedusaContainer } from "@medusajs/framework/types"
import type { AuthenticatedMedusaRequest, MedusaResponse, MedusaNextFunction } from "@medusajs/framework/http"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"
import { hasWarehousePermission, warehouseDeviceResource, assertWarehousePermission, assertWarehouseFulfillment, assertWarehouseLocation, assertWarehouseOrder, assertWarehouseSource, scopeInventoryLevels, scopeWarehouseOrder, warehouseAccess, warehouseDenied, warehouseOrders } from "../lib/warehouse-access"
import type { DeviceCommandBody } from "./admin/devices/validators"

export async function authorizeWarehouseCommand(container: MedusaContainer, actorId: string, body: DeviceCommandBody) {
  const access = await warehouseAccess(container, actorId)
  if (!access.warehouse) return
  const resource = ["save_allocation", "assign", "order_assign", "release", "ship"].includes(body.action) ? "warehouse_allocation"
    : ["receive", "inspect"].includes(body.action) ? "warehouse_return" : "warehouse_intake"
  if (!["intake", "update", "activate", "delete", "retire", "inspect", "make_available"].includes(body.action)) await assertWarehousePermission(container, actorId, resource, "update")
  const devices = container.resolve<DevicesModuleService>(DEVICES_MODULE)
  const ownDevice = async (id: string) => {
    const device = await devices.retrieveDevice(id)
    assertWarehouseLocation(access, device.location_id)
    if (["update", "activate", "delete", "retire", "inspect", "make_available"].includes(body.action)) await assertWarehousePermission(container, actorId, warehouseDeviceResource(device.category), "update")
    return device
  }
  const ownAssignment = async (id: string) => {
    const assignment = await devices.retrieveDeviceAssignment(id)
    await ownDevice(assignment.device_id)
    if (assignment.fulfillment_id) await assertWarehouseFulfillment(container, access, assignment.fulfillment_id, assignment.order_id)
    else await assertWarehouseOrder(container, access, assignment.order_id)
    return assignment
  }
  switch (body.action) {
    case "intake":
      for (const device of body.devices) {
        await assertWarehousePermission(container, actorId, warehouseDeviceResource(device.category), "update")
        await assertWarehouseSource(container, access, device.source_id)
      }
      return
    case "update":
      await ownDevice(body.device_id)
      await assertWarehousePermission(container, actorId, warehouseDeviceResource(body.category), "update")
      assertWarehouseLocation(access, body.location_id)
      await assertWarehouseSource(container, access, body.source_id)
      return
    case "activate": case "delete": case "retire": case "inspect":
      await ownDevice(body.device_id)
      return
    case "make_available":
      for (const id of body.device_ids) await ownDevice(id)
      return
    case "save_allocation":
      await assertWarehouseOrder(container, access, body.order_id)
      for (const allocation of body.allocations) {
        if (!allocation.fulfillment_id) throw warehouseDenied()
        const fulfillment = await assertWarehouseFulfillment(container, access, allocation.fulfillment_id, body.order_id)
        const device = await ownDevice(allocation.device_id)
        // Warehouse staff cannot move a fulfillment to a different warehouse.
        if (device.location_id !== fulfillment.location_id || !fulfillment.items.some((item) => item.line_item_id === allocation.item_id)) throw warehouseDenied()
        if (allocation.assignment_id) await ownAssignment(allocation.assignment_id)
      }
      return
    case "assign": case "order_assign":
      // Use fulfillment-specific allocation so a shared order cannot expose another parcel.
      throw warehouseDenied()
    case "release":
      await ownAssignment(body.assignment_id)
      return
    case "receive":
      await ownAssignment(body.assignment_id)
      await assertWarehouseSource(container, access, body.source_id)
      return
    case "ship":
      await assertWarehouseFulfillment(container, access, body.fulfillment_id)
      return
    default:
      throw warehouseDenied()
  }
}

// Fail closed for unrelated APIs; native list/detail scoping runs in route wrappers after validation.
export async function guardWarehouseAdmin(req: AuthenticatedMedusaRequest, res: MedusaResponse, next: MedusaNextFunction) {
  if (!req.auth_context?.actor_id || req.auth_context.actor_type === "api-key") return next()
  const access = await warehouseAccess(req.scope, req.auth_context.actor_id)
  if (!access.warehouse) return next()
  const path = req.originalUrl.split("?")[0].replace(/\/$/, "")
  const get = req.method === "GET"
  if (path === "/admin/warehouse-access/me") return next()
  if (path === "/admin/devices" || path === "/admin/devices/variants" || path === "/admin/devices/mobile-intake" || path === "/admin/devices/photos") {
    const category = get ? req.query.category : (req.body as { category?: string })?.category
    if (get && category === "recycled") throw warehouseDenied()
    if (get || path !== "/admin/devices") {
      if (category) await assertWarehousePermission(req.scope, req.auth_context.actor_id, warehouseDeviceResource(String(category)), get ? "read" : "update")
      else if (!await hasWarehousePermission(req.scope, req.auth_context.actor_id, "warehouse_intake", get ? "read" : "update") && !await hasWarehousePermission(req.scope, req.auth_context.actor_id, "warehouse_return", get ? "read" : "update")) throw warehouseDenied()
    }
    return next()
  }
  if (get && ["/admin/users/me", "/admin/feature-flags", "/admin/plugins", "/admin/rbac/me/permissions", "/admin/stores", "/admin/return-reasons", "/admin/regions", "/admin/sales-channels"].includes(path)) return next()
  if (get && /^\/admin\/layouts\//.test(path)) return next()
  // Product definitions/SKUs are shared; warehouse inventory levels are still scoped.
  if (get && /^\/admin\/(products|product-variants|product-types|product-tags|product-categories|product-collections)(\/|$)/.test(path)) {
    // Shared definitions do not grant arbitrary traversal into linked warehouse data.
    if (!/^\/admin\/(products|product-variants|product-types|product-tags|product-categories|product-collections)(?:\/[^/]+)?$/.test(path)) throw warehouseDenied()
    if (typeof req.query.fields === "string") {
      const sharedRelations = /^(?:options(?:\.values)?|variants(?:\.(?:options|prices|inventory_items(?:\.inventory(?:\.location_levels)?)?))?|inventory_items(?:\.inventory(?:\.location_levels)?)?|product|prices|images|tags|type|collection|categories|parent_category|category_children)\.[^.*]+$|^(?:options(?:\.values)?|variants(?:\.(?:options|prices|inventory_items(?:\.inventory(?:\.location_levels)?)?))?|inventory_items(?:\.inventory(?:\.location_levels)?)?|product|prices|images|tags|type|collection|categories|parent_category|category_children)\.\*$/
      for (const field of req.query.fields.split(",")) {
        const value = field.replace(/^[+*\-]+/, "")
        if (value.includes(".") && !sharedRelations.test(value)) throw warehouseDenied()
      }
    }
    const json = res.json.bind(res)
    res.json = ((body: any) => {
      if (body.variant) body.variant = scopeInventoryLevels(body.variant, access)
      if (body.variants) body.variants = body.variants.map((variant) => scopeInventoryLevels(variant, access))
      for (const product of [...(body.products || []), ...(body.product ? [body.product] : [])]) if (product.variants) product.variants = product.variants.map((variant) => scopeInventoryLevels(variant, access))
      return json(body)
    }) as typeof res.json
    return next()
  }
  if (get && (path === "/admin/orders" || /^\/admin\/orders\/[^/]+$/.test(path) || /^\/admin\/stock-locations(?:\/[^/]+)?$/.test(path))) return next()
  if (get && /^\/admin\/orders\/[^/]+\/after-sales$/.test(path)) {
    const order = await assertWarehouseOrder(req.scope, access, path.split("/")[3])
    const ids = new Set(scopeWarehouseOrder(order, access).fulfillments?.flatMap((f) => f.items?.map((item) => item.line_item_id) || []))
    const json = res.json.bind(res)
    res.json = ((body: any) => json({ requests: (body.requests || []).map((request) => ({ ...request, items: request.items?.filter((item) => ids.has(item.item_id)) })).filter((request) => request.items?.length) })) as typeof res.json
    return next()
  } else if (get && ["/admin/returns", "/admin/claims", "/admin/exchanges", "/admin/reservations", "/admin/notifications"].includes(path)) {
    const key = path.split("/").pop()!
    return res.json({ [key]: [], count: 0, offset: 0, limit: 50 })
  } else if (get && /^\/admin\/orders\/[^/]+\/changes$/.test(path)) {
    await assertWarehouseOrder(req.scope, access, path.split("/")[3])
    return res.json({ order_changes: [], count: 0, offset: 0, limit: 50 })
  } else throw warehouseDenied()
}

export async function scopeWarehouseOrderRequest(req: AuthenticatedMedusaRequest, res: MedusaResponse, detail = false) {
  const access = await warehouseAccess(req.scope, req.auth_context.actor_id)
  if (!access.warehouse) return
  await assertWarehousePermission(req.scope, req.auth_context.actor_id, "order", "read")
  if (detail) await assertWarehouseOrder(req.scope, access, req.params.id)
  else {
    const orders = await warehouseOrders(req.scope, access)
    req.filterableFields = { $and: [req.filterableFields || {}, { id: orders.length ? orders.map((order) => order.id) : ["__no_order__"] }] }
  }
  req.queryConfig.fields = [...new Set([...req.queryConfig.fields.filter((field) => {
    const value = field.replace(/^\*/, "")
    return !value.includes(".") || /^(items\.(variant(\.(product|options|inventory_items\.(inventory(\.location_levels)?|required_quantity)))?|detail)|fulfillments(\.(items|labels|shipping_option\.service_zone\.fulfillment_set))?|payment_collections)$/.test(value.replace(/\.\*$/, "")) || ["region.automatic_taxes", "payment_collections.status"].includes(value)
  }), "fulfillments.id", "fulfillments.location_id", "fulfillments.items.line_item_id", "fulfillments.items.quantity", "items.*"])]
  const json = res.json.bind(res)
  res.json = ((body: any) => {
    if (body.orders) body.orders = body.orders.map((order) => scopeWarehouseOrder(order, access))
    if (body.order) body.order = scopeWarehouseOrder(body.order, access)
    return json(body)
  }) as typeof res.json
}
