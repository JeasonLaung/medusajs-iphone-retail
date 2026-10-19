import type { ExecArgs } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { updateProductVariantsWorkflow } from "@medusajs/medusa/core-flows"
import { strict as assert } from "node:assert"
import { mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { resolve } from "node:path"

// Explicit one-off repricing, using the imported model starting prices.
// Run without RESET_IPHONE_PRICES=apply to preview without modifying prices.
const POLICY = "storage-battery-condition-v1"
const STORAGE_STEP_EUR = 50
const BATTERY_UPGRADE_EUR = 30
const CONDITION_UPGRADE_EUR = 30

export default async function resetIphonePrices({ container }: ExecArgs) {
  const { products: catalogue } = JSON.parse(readFileSync(resolve(process.cwd(), "../../../imports/swappie-iphone-catalog.json"), "utf8")) as {
    products: { slug: string; price_eur: number; available_storages_gb: number[]; available_colors: string[] }[]
  }
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const fields = [
    "id", "handle", "variants.id", "variants.sku", "variants.metadata",
    "variants.options.value", "variants.options.option.title",
    "variants.price_set.prices.*", "variants.price_set.prices.price_rules.*",
  ]
  const filters = { handle: catalogue.map((item) => `swappie-${item.slug}`) }
  const { data: products } = await query.graph({ entity: "product", fields, filters, pagination: { take: catalogue.length } })
  assert.equal(products.length, catalogue.length, "Missing catalogue products")
  const plans = products.flatMap((product) => {
    const item = catalogue.find((item) => product.handle === `swappie-${item.slug}`)!
    const minimumStorage = Math.min(...item.available_storages_gb)
    assert(Number.isFinite(item.price_eur) && item.price_eur > 0)
    let configurations = 0
    const rows = (product.variants ?? []).map((variant) => {
      assert(variant)
      const options = Object.fromEntries((variant.options ?? []).map((option) => [option?.option?.title, option?.value]))
      const complete = !!(options.Storage && options.Condition && options.Battery && options.Color)
      // Original imported SKUs only have storage; retain them at the model base.
      assert(complete || (!options.Condition && !options.Battery && !options.Color), `Incomplete options: ${variant.sku}`)
      let amount = item.price_eur
      if (complete) {
        configurations++
        const storage = Number(String(options.Storage).replace(/\s*GB$/i, ""))
        assert(item.available_storages_gb.includes(storage), `Unknown capacity: ${variant.sku}`)
        assert(["85%", "100%"].includes(String(options.Battery)), `Unknown battery: ${variant.sku}`)
        assert(["A", "S"].includes(String(options.Condition)), `Unknown condition: ${variant.sku}`)
        assert(item.available_colors.includes(String(options.Color)), `Unknown color: ${variant.sku}`)
        amount += Math.log2(storage / minimumStorage) * STORAGE_STEP_EUR
        amount += options.Battery === "100%" ? BATTERY_UPGRADE_EUR : 0
        amount += options.Condition === "S" ? CONDITION_UPGRADE_EUR : 0
      }
      assert(Number.isSafeInteger(amount) && amount > 0)
      const prices = variant.price_set?.prices ?? []
      // Refuse to replace complex merchant prices with a simple demo policy.
      assert(prices.every((price) => price && price.currency_code === "eur" && !price.price_list_id && !price.min_quantity && !price.max_quantity && !(price as typeof price & { price_rules?: unknown[] }).price_rules?.length), `Complex pricing requires review: ${variant.sku}`)
      assert(prices.length <= 1, `Multiple base prices: ${variant.sku}`)
      return { id: variant.id, sku: variant.sku, options, amount, previous_prices: prices, previous_metadata: variant.metadata }
    })
    assert.equal(configurations, item.available_storages_gb.length * item.available_colors.length * 4, `Incomplete configuration matrix: ${product.handle}`)
    console.log(`${product.handle}: ${rows.length} SKUs, EUR ${Math.min(...rows.map((row) => row.amount))}-${Math.max(...rows.map((row) => row.amount))}`)
    return rows
  })
  const changed = plans.filter((plan) => plan.previous_prices.length !== 1 || Number(plan.previous_prices[0]?.amount) !== plan.amount || plan.previous_metadata?.price_pending !== false || plan.previous_metadata?.demo_price_basis !== POLICY)
  console.log(`${plans.length} SKUs checked; ${changed.length} need repricing. Storage doubling +EUR 50; battery 100% +EUR 30; condition S +EUR 30; color +EUR 0.`)
  if (process.env.RESET_IPHONE_PRICES !== "apply") return

  const backupDirectory = resolve(process.cwd(), "../../../backups")
  mkdirSync(backupDirectory, { recursive: true })
  const backupPath = resolve(backupDirectory, `iphone-prices-${new Date().toISOString().replace(/[:.]/g, "-")}.json`)
  writeFileSync(backupPath, JSON.stringify({ policy: POLICY, created_at: new Date().toISOString(), plans }, null, 2), { flag: "wx" })
  console.log(`Previous prices saved: ${backupPath}`)
  for (let offset = 0; offset < changed.length; offset += 50) {
    await updateProductVariantsWorkflow(container).run({ input: { product_variants: changed.slice(offset, offset + 50).map((plan) => ({
      id: plan.id,
      prices: [{ currency_code: "eur", amount: plan.amount }],
      metadata: { ...plan.previous_metadata, price_pending: false, demo_data: true, demo_price_basis: POLICY },
    })) } })
    console.log(`Repriced ${Math.min(offset + 50, changed.length)}/${changed.length}`)
  }
  const { data: verified } = await query.graph({ entity: "product", fields, filters, pagination: { take: catalogue.length } })
  const expected = new Map(plans.map((plan) => [plan.id, plan]))
  let checked = 0
  for (const product of verified) {
    const groups = new Map<string, { storage: number; battery: number; condition: number; amount: number }>()
    for (const variant of product.variants ?? []) {
      assert(variant)
      const plan = expected.get(variant.id)!
      assert(plan)
      const prices = variant.price_set?.prices ?? []
      assert.equal(prices.length, 1, `Unexpected price count: ${variant.sku}`)
      assert.equal(Number(prices[0]?.amount), plan.amount, `Price mismatch: ${variant.sku}`)
      assert.equal(variant.metadata?.price_pending, false)
      checked++
      if (!plan.options.Color) continue
      const key = [plan.options.Storage, plan.options.Battery, plan.options.Condition].join("|")
      const row = { storage: Number(String(plan.options.Storage).replace(/\s*GB$/i, "")), battery: Number(String(plan.options.Battery).replace("%", "")), condition: plan.options.Condition === "S" ? 1 : 0, amount: plan.amount }
      if (groups.has(key)) assert.equal(groups.get(key)!.amount, row.amount, "Color changes price")
      groups.set(key, row)
    }
    // Every upgrade must increase price while all other attributes are fixed.
    for (const a of groups.values()) for (const b of groups.values()) {
      if (a.storage > b.storage && a.battery === b.battery && a.condition === b.condition) assert(a.amount > b.amount)
      if (a.battery > b.battery && a.storage === b.storage && a.condition === b.condition) assert(a.amount > b.amount)
      if (a.condition > b.condition && a.storage === b.storage && a.battery === b.battery) assert(a.amount > b.amount)
    }
  }
  assert.equal(checked, plans.length)
  const search = container.resolve(Modules.SEARCH)
  await search.ingest({ name: "product.updated", data: products.map((product) => ({ id: product.id })) } as never)
  console.log(`VERIFIED: ${checked} SKU prices, all capacity/battery/condition upgrades increase prices, all colors share prices; search refreshed.`)
}
