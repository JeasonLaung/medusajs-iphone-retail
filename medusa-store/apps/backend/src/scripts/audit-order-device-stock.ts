import type { ExecArgs, IInventoryService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"

export default async function audit({ container }: ExecArgs) {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const inventory: IInventoryService = container.resolve(Modules.INVENTORY)
  const { data: orders } = await query.graph({ entity: "order", filters: { id: "order_01M5EMDP5968NC6TAVRP5V6KFA" }, fields: ["id", "status", "items.id", "items.variant_id", "items.quantity"] })
  console.log("ORDER_STOCK_AUDIT", JSON.stringify(orders))
  for (const item of orders[0]?.items ?? []) {
    if (!item?.variant_id) continue
    const { data: variants } = await query.graph({ entity: "product_variant", filters: { id: item.variant_id }, fields: ["id", "sku", "metadata", "manage_inventory", "allow_backorder", "inventory_items.inventory_item_id"] })
    console.log("VARIANT_STOCK_AUDIT", JSON.stringify({ variants, managed: await service.listDeviceVariants({ variant_id: item.variant_id }), devices: (await service.listDevices({ variant_id: item.variant_id })).map(d => ({ id: d.id, state: d.state, category: d.category, source_id: d.source_id, location_id: d.location_id })), supplies: await service.listDeviceSupplies({ variant_id: item.variant_id }), reservations: await inventory.listReservationItems({ line_item_id: item.id }), levels: await inventory.listInventoryLevels({ inventory_item_id: variants[0]?.inventory_items?.map(i => i!.inventory_item_id!) ?? [] }) }))
  }
  const managed = await service.listDeviceVariants({}, { take: null })
  const levels = await inventory.listInventoryLevels({}, { take: null })
  console.log("CATALOG_STOCK_AUDIT", JSON.stringify({ managed: managed.length, positiveUnmanagedLevels: levels.filter(l => Number(l.stocked_quantity) > 0 && !managed.some(m => m.inventory_item_id === l.inventory_item_id)).length }))
}
