import type { HttpTypes } from "@medusajs/types"
import type { InstantSearchSearchResponse } from "@medusajs/instantsearch-adapter"
import { completeVariants } from "./product-variants"

export function productCardStock(product?: HttpTypes.StoreProduct) {
  const storage = new Set<string>()
  const colors = new Set<string>()
  const options = new Map((product?.options ?? []).map((option) => [option.id, option.title?.trim() ?? ""]))

  // Only complete purchase combinations with positive sellable quantity count.
  // Existing SKUs and model metadata alone say nothing about current stock.
  for (const variant of product ? completeVariants(product) : []) {
    if ((variant.inventory_quantity ?? 0) <= 0) continue
    for (const option of variant.options ?? []) {
      const title = options.get(option.option_id!) ?? ""
      const value = option.value?.trim()
      if (!value) continue
      if (/^(storage|capacity|容量)$/i.test(title)) storage.add(value)
      if (/^(colou?r|颜色)$/i.test(title)) colors.add(value)
    }
  }

  return {
    stocked_storage_options: Array.from(storage),
    stocked_color_options: Array.from(colors),
  }
}

export async function refreshProductCardStock(
  results: InstantSearchSearchResponse[],
  loadProducts: (ids: string[]) => Promise<HttpTypes.StoreProduct[]>
) {
  const ids = Array.from(new Set(results.flatMap((result) => result.hits.map((hit) => hit.objectID))))
  if (!ids.length) return results
  const products = new Map((await loadProducts(ids)).map((product) => [product.id, product]))
  return results.map((result) => ({
    ...result,
    hits: result.hits.map((hit) => ({ ...hit, ...productCardStock(products.get(hit.objectID)) })),
  }))
}
