import type { IInventoryService, MedusaContainer } from "@medusajs/framework/types"
import { Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"

type DeviceStock = { variant_id: string; location_id: string; state: string; category: string }
type SupplyStock = { variant_id: string; source_id: string; quantity: number }
type Source = { id: string; location_id: string; type: string }

// A supplier's registered devices describe the same supply, not additional units.
// Packed/shipped devices have already consumed native stock at fulfillment.
export function deviceStockCaps(devices: DeviceStock[], supplies: SupplyStock[], sources: Source[]) {
  const caps = new Map<string, number>()
  for (const device of devices) {
    if (device.category !== "sale" || !["available", "reserved"].includes(device.state)) continue
    const key = `${device.variant_id}:${device.location_id}`
    caps.set(key, (caps.get(key) ?? 0) + 1)
  }
  for (const supply of supplies) {
    const source = sources.find(entry => entry.id === supply.source_id && entry.type === "supplier")
    if (!source) continue
    const key = `${supply.variant_id}:${source.location_id}`
    caps.set(key, Math.max(caps.get(key) ?? 0, Number(supply.quantity)))
  }
  return caps
}

export async function loadDeviceStock(container: MedusaContainer, variantIds?: string[]) {
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const inventory: IInventoryService = container.resolve(Modules.INVENTORY)
  const managed = await service.listDeviceVariants(variantIds ? { variant_id: variantIds } : {}, { take: null })
  if (!managed.length) return { managed, levels: [], caps: new Map<string, number>() }
  const ids = managed.map(row => row.variant_id)
  const [devices, supplies, sources, levels] = await Promise.all([
    service.listDevices({ variant_id: ids, state: ["available", "reserved"], category: "sale" }, { take: null }),
    service.listDeviceSupplies({ variant_id: ids }, { take: null }),
    service.listDeviceSources({}, { take: null }),
    inventory.listInventoryLevels({ inventory_item_id: managed.map(row => row.inventory_item_id) }, { take: null }),
  ])
  return { managed, levels, caps: deviceStockCaps(devices, supplies, sources) }
}
