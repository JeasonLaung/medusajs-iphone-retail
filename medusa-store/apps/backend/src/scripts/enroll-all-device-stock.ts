import type { ExecArgs, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, MedusaError, Modules } from "@medusajs/framework/utils"
import { mkdirSync, writeFileSync } from "node:fs"
import { resolve } from "node:path"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"
import { enrollAllDeviceStockWorkflow } from "../workflows/enroll-all-device-stock"

export default async function enroll({ container }: ExecArgs) {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const { data: variants } = await query.graph({ entity: "product_variant", fields: ["id", "sku", "manage_inventory", "allow_backorder", "inventory_items.*", "inventory_items.inventory.location_levels.*"], pagination: { take: 100000 } })
  const managed = await service.listDeviceVariants({}, { take: null })
  console.log(`All SKUs: ${variants.length}; managed: ${managed.length}; pending: ${variants.length - managed.length}`)
  if (process.env.ENROLL_ALL_DEVICE_STOCK !== "apply") return
  const users: IUserModuleService = container.resolve(Modules.USER)
  const actor = (await users.listUsers({})).find(u => u.metadata?.device_role !== "supplier")
  if (!actor) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "Administrator required")
  const backupDir = resolve(process.cwd(), "../../../backups")
  mkdirSync(backupDir, { recursive: true })
  const backup = resolve(backupDir, `device-stock-${new Date().toISOString().replace(/[:.]/g, "-")}.json`)
  writeFileSync(backup, JSON.stringify({ variants, managed }, null, 2))
  console.log(`Backup: ${backup}`)
  const { result } = await enrollAllDeviceStockWorkflow(container).run({ input: { actor_id: actor.id, preserve_reservations: process.env.PRESERVE_DEVICE_RESERVATIONS === "true" } })
  const after = await service.listDeviceVariants({}, { take: null })
  if (variants.some(v => !after.some(m => m.variant_id === v.id))) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, "Some SKUs remain unmanaged")
  console.log(`VERIFIED: ${after.length} managed SKUs; ${result.enrolled} newly enrolled`)
}
