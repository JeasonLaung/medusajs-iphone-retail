import type { SearchTypes } from "@medusajs/framework/types"
import { getVariantAvailability } from "@medusajs/framework/utils"

type StockProduct = {
  id: string
  sales_channels?: ({ id?: string | null } | null)[] | null
  variants?: ({ id?: string; options?: ({ value?: string | null; option?: { title?: string | null } | null } | null)[] | null } | null)[] | null
}

export function stockedStorageOptions(product: StockProduct, stockedIds: Set<string>) {
  return Array.from(new Set((product.variants ?? []).flatMap((variant) => {
    if (!variant?.id || !stockedIds.has(variant.id)) return []
    return (variant.options ?? []).flatMap((option) => {
      const value = option?.value?.trim()
      return value && /^(storage|capacity|容量)$/i.test(option?.option?.title?.trim() ?? "") ? [value] : []
    })
  })))
}

// Use sellable quantity after reservations, restricted to linked sales-channel
// locations. SKU presence, backorders and unmanaged inventory do not imply stock.
export async function loadStockedVariantIds(products: StockProduct[], { container }: SearchTypes.SearchIngestionContext) {
  const byChannel = new Map<string, Set<string>>()
  for (const product of products) {
    for (const channel of product.sales_channels ?? []) {
      if (!channel?.id) continue
      const ids = byChannel.get(channel.id) ?? new Set<string>()
      for (const variant of product.variants ?? []) {
        if (variant?.id) ids.add(variant.id)
      }
      byChannel.set(channel.id, ids)
    }
  }
  const stocked = new Set<string>()
  await Promise.all(Array.from(byChannel, async ([channel, ids]) => {
    if (!ids.size) return
    const availability = await getVariantAvailability(container.query, {
      variant_ids: Array.from(ids), sales_channel_id: channel,
    })
    for (const [id, stock] of Object.entries(availability)) {
      if ((stock.availability ?? 0) > 0) stocked.add(id)
    }
  }))
  return stocked
}
