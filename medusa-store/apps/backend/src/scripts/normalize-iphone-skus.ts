import type { ExecArgs } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { updateInventoryItemsWorkflow, updateProductVariantsWorkflow } from "@medusajs/medusa/core-flows"
import { strict as assert } from "node:assert"
import { mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { resolve } from "node:path"

// Preview by default. Set NORMALIZE_IPHONE_SKUS=apply to update existing data.
export default async function normalizeIphoneSkus({ container }: ExecArgs) {
  const { products: catalogue } = JSON.parse(readFileSync(resolve(process.cwd(), "../../../imports/swappie-iphone-catalog.json"), "utf8")) as {
    products: { slug: string }[]
  }
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const fields = [
    "id", "handle", "variants.id", "variants.sku", "variants.metadata",
    "variants.price_set.prices.*", "variants.manage_inventory", "variants.allow_backorder",
    "variants.inventory_items.inventory_item_id", "variants.inventory_items.required_quantity",
    "variants.inventory_items.inventory.sku", "variants.inventory_items.inventory.location_levels.*",
  ]
  const filters = { handle: catalogue.map((item) => `swappie-${item.slug}`) }
  const load = async () => {
    const { data } = await query.graph({ entity: "product", fields, filters, pagination: { take: catalogue.length } })
    assert.equal(data.length, catalogue.length, "Missing catalogue products")
    return data
  }
  const products = await load()
  const variants = products.flatMap((product) => product.variants ?? []).filter((variant) => variant != null)
  variants.sort((a, b) => a.id.localeCompare(b.id))
  const valid = (sku: string | null | undefined) => /^ERP[1-9][0-9]{4,}$/.test(sku ?? "") && BigInt(sku!.slice(3)) > 10000n
  const pending = variants.filter((variant) => !valid(variant.sku))
  const inventoryIds = variants.flatMap((variant) => (variant.inventory_items ?? []).map((item) => item?.inventory_item_id))
  assert.equal(new Set(inventoryIds).size, inventoryIds.length, "Shared inventory items require review")
  for (const variant of variants) assert.equal(variant.inventory_items?.length, 1, `Expected one inventory item: ${variant.id}`)
  console.log(`${products.length} iPhone models, ${variants.length} variants; ${pending.length} need ERP + (ID + 10000) SKUs.`)
  if (process.env.NORMALIZE_IPHONE_SKUS !== "apply") return

  const directory = resolve(process.cwd(), "../../../backups")
  mkdirSync(directory, { recursive: true })
  const backup = resolve(directory, `iphone-skus-${new Date().toISOString().replace(/[:.]/g, "-")}.json`)
  writeFileSync(backup, JSON.stringify({ rule: "ERP + (ID + 10000)", products }, null, 2), { flag: "wx" })
  console.log(`Previous SKUs and related data saved: ${backup}`)

  for (let offset = 0; offset < pending.length; offset += 50) {
    await updateProductVariantsWorkflow(container).run({ input: { product_variants: pending.slice(offset, offset + 50).map((variant) => ({
      id: variant.id,
      sku: "",
      metadata: { ...variant.metadata, source_sku: variant.metadata?.source_sku ?? variant.sku },
    })) } })
    console.log(`Renamed ${Math.min(offset + 50, pending.length)}/${pending.length}`)
  }

  const renamed = (await load()).flatMap((product) => product.variants ?? []).filter((variant) => variant != null)
  const updates = renamed.flatMap((variant) => {
    assert(valid(variant.sku), `Invalid ERP SKU after update: ${variant.id}`)
    return (variant.inventory_items ?? []).flatMap((item) => item?.inventory?.sku === variant.sku ? [] : [{
      id: item!.inventory_item_id!, sku: variant.sku!,
    }])
  })
  for (let offset = 0; offset < updates.length; offset += 50) {
    await updateInventoryItemsWorkflow(container).run({ input: { updates: updates.slice(offset, offset + 50) } })
  }

  const verified = (await load()).flatMap((product) => product.variants ?? []).filter((variant) => variant != null)
  assert.equal(verified.length, variants.length)
  assert.equal(new Set(verified.map((variant) => variant.sku)).size, variants.length)
  const original = new Map(variants.map((variant) => [variant.id, variant]))
  for (const variant of verified) {
    const previous = original.get(variant.id)!
    assert(previous, `Unexpected variant: ${variant.id}`)
    assert(valid(variant.sku))
    assert.equal(variant.manage_inventory, previous.manage_inventory)
    assert.equal(variant.allow_backorder, previous.allow_backorder)
    assert.deepEqual(variant.price_set, previous.price_set, `Prices changed: ${variant.id}`)
    const inventory = variant.inventory_items![0]!
    const oldInventory = previous.inventory_items![0]!
    assert.equal(inventory.inventory_item_id, oldInventory.inventory_item_id)
    assert.equal(inventory.required_quantity, oldInventory.required_quantity)
    assert.equal(inventory.inventory?.sku, variant.sku)
    assert.deepEqual(inventory.inventory?.location_levels, oldInventory.inventory?.location_levels, `Stock changed: ${variant.id}`)
    if (valid(previous.sku)) assert.equal(variant.sku, previous.sku, "Generated SKU was reassigned")
    else assert.equal(variant.metadata?.source_sku, previous.metadata?.source_sku ?? previous.sku)
  }
  const search = container.resolve(Modules.SEARCH)
  await search.ingest({ name: "product.updated", data: products.map((product) => ({ id: product.id })) } as never)
  console.log(`VERIFIED: ${verified.length} unique ERP SKUs; inventory SKUs synchronized; prices, stock and variant IDs preserved; search refreshed.`)
}
