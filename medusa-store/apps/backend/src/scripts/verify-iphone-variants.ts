import type { ExecArgs } from "@medusajs/framework/types"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { strict as assert } from "node:assert"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

export default async function verifyIphoneVariants({ container }: ExecArgs) {
  const catalogue = JSON.parse(readFileSync(resolve(process.cwd(), "../../../imports/swappie-iphone-catalog.json"), "utf8")) as {
    products: { slug: string; sku: string; available_storages_gb: number[]; available_colors: string[] }[]
  }
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data: products } = await query.graph({
    entity: "product",
    filters: { handle: catalogue.products.map((product) => `swappie-${product.slug}`) },
    fields: ["id", "handle", "options.title", "options.values.value", "variants.id", "variants.sku", "variants.metadata", "variants.manage_inventory", "variants.allow_backorder", "variants.options.value", "variants.options.option.title", "variants.inventory_items.inventory_item_id", "variants.inventory_items.inventory.location_levels.stocked_quantity", "variants.inventory_items.inventory.location_levels.reserved_quantity"],
    pagination: { take: catalogue.products.length },
  })
  assert.equal(products.length, catalogue.products.length)
  let combinations = 0
  let stocked = 0
  for (const item of catalogue.products) {
    const product = products.find((product) => product.handle === `swappie-${item.slug}`)!
    assert(product.variants?.some((variant) => variant?.sku === item.sku), `Original SKU missing: ${item.sku}`)
    const expected = item.available_storages_gb.length * item.available_colors.length * 4
    const variants = (product.variants ?? []).filter((variant) => variant?.metadata?.configuration_source === "catalogue-options")
    assert.equal(variants.length, expected, `Incomplete matrix: ${item.slug}`)
    assert.equal(new Set(variants.map((variant) => variant?.sku)).size, expected)
    for (const variant of variants) {
      assert(variant)
      const values = Object.fromEntries((variant.options ?? []).map((option) => [option?.option?.title, option?.value]))
      const slug = `${item.slug}-${String(values.Storage).replace(/\s+/g, "").toLowerCase()}-${String(values.Color).toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "")}-${values.Condition}${String(values.Battery).replace(/%/g, "")}`
      assert.equal(variant.sku, slug)
      assert.equal(variant.metadata?.slug, slug)
      assert.equal(variant.manage_inventory, true)
      assert.equal(variant.allow_backorder, false)
      assert.equal(variant.inventory_items?.length, 1)
      const quantity = (variant.inventory_items ?? []).reduce((sum, item) => sum + (item?.inventory?.location_levels ?? []).reduce((total, level) => total + Number(level?.stocked_quantity ?? 0) - Number(level?.reserved_quantity ?? 0), 0), 0)
      if (quantity > 0) stocked++
    }
    combinations += expected
  }
  console.log(`VERIFIED: ${products.length} models, ${combinations} unique configuration SKUs, ${stocked} stocked combinations; all original SKUs retained`)
}
