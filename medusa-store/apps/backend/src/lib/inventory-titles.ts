import type { MedusaContainer } from "@medusajs/framework/types"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"

export const inventoryTitle = (productTitle: string, variantTitle: string) => {
  const product = productTitle.trim()
  const variant = variantTitle.trim()
  if (!product || variant === product || variant.startsWith(`${product} / `)) return variant || product
  return variant ? `${product} / ${variant}` : product
}

// Change only default names on inventory dedicated to one variant.
export async function planInventoryTitles(container: MedusaContainer, variantIds?: string[]) {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const candidates = new Map<string, { id: string; title: string; previous: string | null }>()
  let offset = 0
  while (true) {
    const { data: variants } = await query.graph({
      entity: "product_variant",
      fields: ["id", "title", "product.title", "inventory_items.inventory_item_id", "inventory_items.inventory.title"],
      filters: variantIds ? { id: variantIds } : {},
      pagination: { skip: offset, take: 500, order: { id: "ASC" } },
    })
    for (const variant of variants) {
      if (!variant.product?.title || variant.inventory_items?.length !== 1) continue
      const item = variant.inventory_items[0]!
      const previous = item.inventory?.title ?? null
      if (!item.inventory_item_id || (previous?.trim() && previous.trim() !== variant.title?.trim())) continue
      const title = inventoryTitle(variant.product.title, variant.title ?? "")
      if (title !== previous) candidates.set(item.inventory_item_id, { id: item.inventory_item_id, title, previous })
    }
    if (variants.length < 500) break
    offset += variants.length
  }

  const ids = [...candidates.keys()]
  const counts = new Map<string, number>()
  for (let start = 0; start < ids.length; start += 500) {
    let skip = 0
    while (true) {
      const { data: links } = await query.graph({
        entity: "product_variant_inventory_item",
        fields: ["inventory_item_id", "variant_id"],
        filters: { inventory_item_id: ids.slice(start, start + 500) },
        pagination: { skip, take: 500 },
      })
      for (const link of links) {
        if (link.inventory_item_id) counts.set(link.inventory_item_id, (counts.get(link.inventory_item_id) ?? 0) + 1)
      }
      if (links.length < 500) break
      skip += links.length
    }
  }
  return [...candidates.values()].filter(item => counts.get(item.id) === 1)
}
