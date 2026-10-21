import type { ExecArgs } from "@medusajs/framework/types"
import { mkdirSync, writeFileSync } from "node:fs"
import { resolve } from "node:path"
import { loadDeviceStock } from "../lib/device-stock"
import { reconcileDeviceStockWorkflow } from "../workflows/reconcile-device-stock"

export default async function reconcile({ container }: ExecArgs) {
  const snapshot = await loadDeviceStock(container)
  const variants = new Map(snapshot.managed.map(row => [row.inventory_item_id, row.variant_id]))
  const surplus = snapshot.levels.filter(level => Number(level.stocked_quantity) > (snapshot.caps.get(`${variants.get(level.inventory_item_id)}:${level.location_id}`) ?? 0))
  console.log(`Device stock audit: ${snapshot.managed.length} managed SKUs, ${surplus.length} unsupported inventory levels`)
  if (process.env.RECONCILE_DEVICE_STOCK !== "apply") return
  const directory = resolve(process.cwd(), "../../../backups")
  mkdirSync(directory, { recursive: true })
  const backup = resolve(directory, `device-stock-reconciliation-${Date.now()}.json`)
  writeFileSync(backup, JSON.stringify({ managed: snapshot.managed, levels: snapshot.levels, caps: [...snapshot.caps] }, null, 2))
  const { result } = await reconcileDeviceStockWorkflow(container).run({ input: {} })
  console.log(`Device stock reconciled: ${result.updates.length} corrected levels. Backup: ${backup}`)
}
