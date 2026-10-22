import { hasPermission } from "@medusajs/framework"
import type { MedusaContainer } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, MedusaError, Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"

export const WAREHOUSE_ROLE = "role_retail_warehouse"
export type WarehouseAccess = { warehouse: boolean; location_ids: string[] | null }
export const warehouseDenied = () => new MedusaError(MedusaError.Types.FORBIDDEN, "WAREHOUSE_ACCESS_DENIED")

export async function warehouseAccess(container: MedusaContainer, actorId: string): Promise<WarehouseAccess> {
  const [user] = await container.resolve(Modules.USER).listUsers({ id: actorId })
  if (!user) return { warehouse: false, location_ids: null }
  const metadata = user.metadata || {}
  const { data: users }: { data: any[] } = await container.resolve(ContainerRegistrationKeys.QUERY).graph({ entity: "user", fields: ["id", "rbac_roles.id"], filters: { id: actorId } })
  const warehouseRole = users[0]?.rbac_roles?.some((role) => role?.id === WAREHOUSE_ROLE)
  const warehouse = warehouseRole || metadata.device_role === "warehouse" || Array.isArray(metadata.warehouse_location_ids)
  if (!warehouse) return { warehouse: false, location_ids: null }
  return { warehouse: true, location_ids: metadata.warehouse_scope === "all" ? null :
    Array.isArray(metadata.warehouse_location_ids) ? [...new Set(metadata.warehouse_location_ids.filter((id): id is string => typeof id === "string" && Boolean(id)))] : [] }
}

export function assertWarehouseLocation(access: WarehouseAccess, locationId: string) {
  if (access.location_ids !== null && !access.location_ids.includes(locationId)) throw warehouseDenied()
}

export async function assertWarehouseSource(container: MedusaContainer, access: WarehouseAccess, sourceId: string) {
  const source = await container.resolve<DevicesModuleService>(DEVICES_MODULE).retrieveDeviceSource(sourceId)
  assertWarehouseLocation(access, source.location_id)
  if (access.warehouse && source.type !== "self") throw warehouseDenied()
  return source
}

export type WarehouseOrder = { id: string; fulfillments?: any[] | null; items?: any[] | null; [key: string]: any }
export function scopeWarehouseOrder<T extends WarehouseOrder>(order: T, access: WarehouseAccess): T {
  if (access.location_ids === null) return order
  const fulfillments = (order.fulfillments || []).filter((f) => access.location_ids!.includes(f.location_id))
  const quantities = new Map<string, number>()
  for (const fulfillment of fulfillments.filter((f) => !f.canceled_at)) for (const item of fulfillment.items || []) {
    quantities.set(item.line_item_id, (quantities.get(item.line_item_id) || 0) + Number(item.quantity))
  }
  const items = (order.items || []).filter((item) => quantities.has(item.id)).map((item) => ({
    ...item, quantity: quantities.get(item.id),
    variant: item.variant ? scopeInventoryLevels(item.variant, access) : item.variant,
    detail: item.detail ? { quantity: quantities.get(item.id), fulfilled_quantity: quantities.get(item.id) } : item.detail,
  }))
  for (const item of items) for (const key of Object.keys(item)) {
    if (key === "total" || key.endsWith("_total") || key.endsWith("_subtotal")) item[key] = Number(item[key] || 0) * Number(item.quantity) / Number((order.items || []).find((original) => original.id === item.id)?.quantity || 1)
  }
  // Payment/refund records and order-wide totals include other warehouses' sales.
  const result: WarehouseOrder = { ...order, fulfillments, items, payment_collections: order.payment_collections?.map((collection) => ({ status: collection.status })), credit_lines: [], returns: [], claims: [], exchanges: [], summary: undefined,
    metadata: {}, promotions: [], shipping_methods: [],
    customer: order.customer ? { id: order.customer.id, first_name: order.customer.first_name, last_name: order.customer.last_name, email: order.customer.email, phone: order.customer.phone, company_name: order.customer.company_name } : order.customer,
  }
  for (const key of Object.keys(result)) if (key === "total" || key.endsWith("_total") || key.endsWith("_subtotal")) {
    result[key] = items.reduce((sum, item) => sum + Number(item[key] || 0), 0)
  }
  return result as T
}

export async function warehouseOrders(container: MedusaContainer, access: WarehouseAccess): Promise<WarehouseOrder[]> {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const fulfillments = await container.resolve(Modules.FULFILLMENT).listFulfillments(
    { ...(access.location_ids === null ? {} : { location_id: access.location_ids.length ? access.location_ids : ["__no_warehouse__"] }) },
    { take: null, relations: ["items"] },
  )
  if (!fulfillments.length) return []
  const ids = fulfillments.filter((fulfillment) => !fulfillment.canceled_at).map((fulfillment) => fulfillment.id)
  if (!ids.length) return []
  const { data: links }: { data: any[] } = await query.graph({ entity: "order_fulfillment", fields: ["order_id", "fulfillment_id"], filters: { fulfillment_id: ids } })
  const orders: WarehouseOrder[] = []
  for (const entry of links) {
    let order = orders.find((order) => order.id === entry.order_id)
    if (!order) { order = { id: entry.order_id, fulfillments: [] }; orders.push(order) }
    const fulfillment = fulfillments.find((fulfillment) => fulfillment.id === entry.fulfillment_id)
    if (fulfillment) order.fulfillments!.push(fulfillment)
  }
  return orders
}

export async function assertWarehouseFulfillment(container: MedusaContainer, access: WarehouseAccess, fulfillmentId: string, orderId?: string) {
  const fulfillment = await container.resolve(Modules.FULFILLMENT).retrieveFulfillment(fulfillmentId, { relations: ["items"] })
  assertWarehouseLocation(access, fulfillment.location_id)
  if (orderId) {
    const { data } = await container.resolve(ContainerRegistrationKeys.QUERY).graph({ entity: "order", fields: ["id", "fulfillments.id"], filters: { id: orderId } })
    if (!data[0]?.fulfillments?.some((f) => f?.id === fulfillmentId)) throw warehouseDenied()
  }
  return fulfillment
}

export async function assertWarehouseOrder(container: MedusaContainer, access: WarehouseAccess, orderId: string) {
  const order = (await warehouseOrders(container, access)).find((order) => order.id === orderId)
  if (!order) throw warehouseDenied()
  return order
}

export function scopeInventoryLevels(variant: any, access: WarehouseAccess) {
  if (access.location_ids === null) return variant
  const inventoryItems = variant.inventory_items?.map((item: any) => {
    const levels = item.inventory?.location_levels?.filter((level: any) => level && access.location_ids!.includes(level.location_id)) || []
    const stocked = levels.reduce((sum: number, level: any) => sum + Number(level.stocked_quantity || 0), 0)
    const reserved = levels.reduce((sum: number, level: any) => sum + Number(level.reserved_quantity || 0), 0)
    return { ...item, inventory: { ...item.inventory, location_levels: levels, stocked_quantity: stocked, reserved_quantity: reserved } }
  })
  const inventoryQuantity = inventoryItems?.length ? Math.min(...inventoryItems.map((item: any) => (item.inventory.stocked_quantity - item.inventory.reserved_quantity) / Number(item.required_quantity || 1))) : undefined
  return { ...variant, inventory_quantity: inventoryQuantity, inventory_items: inventoryItems }

}

export async function assertWarehousePermission(container: MedusaContainer, actorId: string, resource: string, operation: string) {
  const { data }: { data: any[] } = await container.resolve(ContainerRegistrationKeys.QUERY).graph({ entity: "user", fields: ["id", "rbac_roles.id"], filters: { id: actorId } })
  const roles = data[0]?.rbac_roles?.filter(Boolean).map((role) => role.id) || []
  if (!roles.length || !await hasPermission({ container, roles, actions: { resource, operation } })) throw warehouseDenied()
}
