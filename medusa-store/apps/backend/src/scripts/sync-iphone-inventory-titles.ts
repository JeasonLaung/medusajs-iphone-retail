import type { ExecArgs } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"
import { updateInventoryItemsWorkflow } from "@medusajs/medusa/core-flows"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

type Catalogue = { products: { slug: string }[] }

export default async function syncIphoneInventoryTitles({ container }: ExecArgs) {
  const catalogue = JSON.parse(
    readFileSync(resolve(process.cwd(), "../../../imports/swappie-iphone-catalog.json"), "utf8"),
  ) as Catalogue
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data: products } = await query.graph({
    entity: "product",
    fields: [
      "handle",
      "title",
      "variants.id",
      "variants.title",
      "variants.inventory_items.inventory_item_id",
      "variants.inventory_items.inventory.title",
    ],
    filters: { handle: catalogue.products.map((item) => `swappie-${item.slug}`) },
    pagination: { take: catalogue.products.length },
  })
  if (products.length !== catalogue.products.length) {
    throw new MedusaError(
      MedusaError.Types.INVALID_DATA,
      `Expected ${catalogue.products.length} iPhone products, found ${products.length}`,
    )
  }

  const updates = new Map<string, { id: string; title: string }>()
  for (const product of products) {
    for (const variant of product.variants ?? []) {
      const item = variant.inventory_items?.[0]
      if (!item?.inventory_item_id || variant.inventory_items?.length !== 1) {
        throw new MedusaError(
          MedusaError.Types.INVALID_DATA,
          `Expected one inventory item for variant ${variant.id}`,
        )
      }
      const variantTitle = variant.title?.trim() ?? ""
      const title = variantTitle.startsWith(`${product.title} / `)
        ? variantTitle
        : variantTitle
          ? `${product.title} / ${variantTitle}`
          : product.title
      if (item.inventory?.title?.trim() !== title) {
        updates.set(item.inventory_item_id, { id: item.inventory_item_id, title })
      }
    }
  }

  const planned = [...updates.values()]
  for (let start = 0; start < planned.length; start += 100) {
    await updateInventoryItemsWorkflow(container).run({
      input: { updates: planned.slice(start, start + 100) },
    })
  }
  console.log(`Synchronized ${planned.length} iPhone inventory item titles with model prefixes`)
}
