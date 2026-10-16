import type { ExecArgs } from "@medusajs/framework/types"
import { Modules } from "@medusajs/framework/utils"
import { createAndLinkProductOptionsToProductWorkflow, createProductVariantsWorkflow, updateProductOptionsWorkflow, updateProductVariantsWorkflow } from "@medusajs/medusa/core-flows"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

type Catalogue = { products: { slug: string; available_storages_gb: number[]; available_colors: string[] }[] }
type Supplements = { condition_options: string[]; battery_options: string[] }

// Existing variants, prices, inventory items and reservations are retained.
// New combinations have managed inventory, no backorders and no assumed price.
export default async function completeIphoneVariants({ container }: Pick<ExecArgs, "container">) {
  const catalogue = JSON.parse(readFileSync(resolve(process.cwd(), "../../../imports/swappie-iphone-catalog.json"), "utf8")) as Catalogue
  const supplements = JSON.parse(readFileSync(resolve(process.cwd(), "../../../imports/swappie-filter-supplements.json"), "utf8")) as Supplements
  const service = container.resolve(Modules.PRODUCT)
  let created = 0
  for (const item of catalogue.products) {
    const [product] = await service.listProducts({ handle: `swappie-${item.slug}` }, { relations: ["options.values", "variants.options"] })
    if (!product) continue
    const definitions = [
      { title: "Storage", values: item.available_storages_gb.map((value) => `${value} GB`) },
      { title: "Condition", values: supplements.condition_options },
      { title: "Battery", values: supplements.battery_options },
      { title: "Color", values: item.available_colors },
    ]
    // Preserve any additional merchant-defined options as well.
    for (const option of product.options ?? []) {
      if (!definitions.some((definition) => definition.title === option.title)) {
        throw new Error(`Unexpected option ${option.title} on ${product.handle}; refusing to invent values`)
      }
    }
    for (const definition of definitions) {
      const existing = product.options?.find((option) => option.title === definition.title)
      if (existing) {
        const global = await service.retrieveProductOption(existing.id, { relations: ["values"] })
        const values = Array.from(new Set([...definition.values, ...global.values.map((value) => value.value)]))
        if (values.length !== global.values.length) await updateProductOptionsWorkflow(container).run({ input: { selector: { id: existing.id }, update: { values } } })
        const expanded = await service.retrieveProductOption(existing.id, { relations: ["values"] })
        const add = expanded.values.filter((value) => definition.values.includes(value.value) && !existing.values.some((entry) => entry.id === value.id)).map((value) => value.id)
        if (add.length) await createAndLinkProductOptionsToProductWorkflow(container).run({ input: { product_id: product.id, update: [{ product_option_id: existing.id, add }] } })
      } else {
        await createAndLinkProductOptionsToProductWorkflow(container).run({ input: { product_id: product.id, add: [{ ...definition, is_exclusive: true }] } })
      }
    }
    const [updated] = await service.listProducts({ id: product.id }, { relations: ["options.values", "variants.options"] })
    const optionTitles = new Map(updated.options.map((option) => [option.id, option.title]))
    const slug = (values: Record<string, string>) => `${item.slug}-${values.Storage.replace(/\s+/g, "").toLowerCase()}-${values.Color.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "")}-${values.Condition}${values.Battery.replace(/%/g, "")}`
    const normalized = updated.variants.flatMap((variant) => {
      const values = Object.fromEntries((variant.options ?? []).map((option) => [optionTitles.get(option.option_id ?? "") ?? "", option.value]))
      if (!definitions.every((option) => values[option.title])) return []
      const canonical = slug(values)
      return variant.sku === canonical && variant.metadata?.slug === canonical ? [] : [{
        id: variant.id, sku: canonical, metadata: { ...variant.metadata, slug: canonical },
      }]
    })
    for (let index = 0; index < normalized.length; index += 50) {
      await updateProductVariantsWorkflow(container).run({ input: { product_variants: normalized.slice(index, index + 50) } })
    }
    const key = (values: Record<string, string>) => definitions.map((option) => values[option.title] ?? "").join("|")
    const existingKeys = new Set(updated.variants.map((variant) => key(Object.fromEntries((variant.options ?? []).map((option) => [optionTitles.get(option.option_id ?? "") ?? "", option.value])))))
    const existingSkus = new Set(updated.variants.map((variant) => variant.sku))
    const combinations = definitions.reduce<Record<string, string>[]>((rows, option) => rows.flatMap((row) => option.values.map((value) => ({ ...row, [option.title]: value }))), [{}])
    const missing = combinations.filter((values) => !existingKeys.has(key(values))).map((values) => ({
      product_id: product.id,
      title: definitions.map((option) => values[option.title]).join(" / "),
      sku: slug(values),
      options: values,
      manage_inventory: true,
      allow_backorder: false,
      metadata: { configuration_source: "catalogue-options", price_pending: true, slug: slug(values) },
    }))
    if (new Set(missing.map((variant) => variant.sku)).size !== missing.length || missing.some((variant) => existingSkus.has(variant.sku))) {
      throw new Error(`SKU collision for ${product.handle}`)
    }
    for (let index = 0; index < missing.length; index += 50) {
      await createProductVariantsWorkflow(container).run({ input: { product_variants: missing.slice(index, index + 50) } })
    }
    created += missing.length
    console.log(`${product.handle}: ${missing.length} combinations created, existing inventory preserved`)
  }
  console.log(`Completed iPhone variants: ${created} created; new inventory is zero and prices await merchant input`)
}
