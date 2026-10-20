import type { ExecArgs, IInventoryService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"
import { strict as assert } from "node:assert"
import { enrollAllDeviceStockWorkflow } from "../workflows/enroll-all-device-stock"

export default async function audit({ container }: ExecArgs) {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const inventory: IInventoryService = container.resolve(Modules.INVENTORY)
  const { data: variants } = await query.graph({ entity: "product_variant", fields: ["id", "sku", "manage_inventory", "allow_backorder", "inventory_items.inventory_item_id", "inventory_items.required_quantity"], pagination: { take: 100000 } })
  const managed = await service.listDeviceVariants({}, { take: null })
  const devices = await service.listDevices({}, { take: null })
  const levels = await inventory.listInventoryLevels({}, { take: null })
  if (process.env.CHECK_DEVICE_STOCK_GUARD === "true") {
    assert.equal(managed.length, 0, "Guard check requires the initial unmanaged catalogue")
    assert(levels.some(l => Number(l.reserved_quantity) > 0), "Guard check requires existing reservations")
    await assert.rejects(() => enrollAllDeviceStockWorkflow(container).run({ input: { actor_id: "stock-migration-check", preserve_reservations: false } }), (error: any) => {
      assert.match(error.message || JSON.stringify(error), /Existing reservation requires review/)
      return true
    })
    assert.equal((await service.listDeviceVariants({}, { take: null })).length, 0)
    const after = await inventory.listInventoryLevels({}, { take: null })
    assert.deepEqual(after.map(l => [l.id, Number(l.stocked_quantity), Number(l.reserved_quantity)]).sort(), levels.map(l => [l.id, Number(l.stocked_quantity), Number(l.reserved_quantity)]).sort())
    console.log("VERIFIED: reservation conflict blocks migration before any inventory or enrolment change")
  }
  console.log(JSON.stringify({ variants: variants.length, managed: managed.length, devices: devices.map(d => ({ id: d.id, variant_id: d.variant_id, location_id: d.location_id, state: d.state })), stock: levels.reduce((n, l) => n + Number(l.stocked_quantity), 0), reserved: levels.filter(l => Number(l.reserved_quantity) > 0).map(l => ({ inventory_item_id: l.inventory_item_id, quantity: l.reserved_quantity })), incompatible: variants.filter(v => !v.manage_inventory || v.allow_backorder || v.inventory_items?.length !== 1 || Number(v.inventory_items[0]?.required_quantity) !== 1).map(v => v.sku) }))
}
