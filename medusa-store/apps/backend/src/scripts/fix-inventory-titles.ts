import type { ExecArgs } from "@medusajs/framework/types"
import { updateInventoryItemsWorkflow } from "@medusajs/medusa/core-flows"
import { mkdirSync, writeFileSync } from "node:fs"
import { resolve } from "node:path"
import { MedusaError } from "@medusajs/framework/utils"
import { planInventoryTitles } from "../lib/inventory-titles"

export default async function fixInventoryTitles({ container }: ExecArgs) {
  const planned = await planInventoryTitles(container)
  console.log(`Inventory titles: ${planned.length} default titles need product names`)
  console.log(JSON.stringify(planned.slice(0, 5), null, 2))
  if (process.env.FIX_INVENTORY_TITLES !== "apply" || !planned.length) return

  const directory = resolve(process.cwd(), "../../../backups")
  mkdirSync(directory, { recursive: true })
  const backup = resolve(directory, `inventory-titles-${Date.now()}.json`)
  writeFileSync(backup, JSON.stringify(planned, null, 2))
  for (let start = 0; start < planned.length; start += 100) {
    await updateInventoryItemsWorkflow(container).run({
      input: { updates: planned.slice(start, start + 100).map(({ id, title }) => ({ id, title })) },
    })
  }
  const remaining = await planInventoryTitles(container)
  if (remaining.length) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, `Inventory title verification failed: ${remaining.length} remaining`)
  console.log(`VERIFIED: ${planned.length} inventory titles updated. Backup: ${backup}`)
}
