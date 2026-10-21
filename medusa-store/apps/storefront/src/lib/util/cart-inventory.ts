import type { HttpTypes } from "@medusajs/types"

export function cartVariantQuantities(items: Pick<HttpTypes.StoreCartLineItem, "variant_id" | "quantity">[]) {
  const quantities: Record<string, number> = {}
  for (const item of items) {
    if (item.variant_id) quantities[item.variant_id] = (quantities[item.variant_id] ?? 0) + item.quantity
  }
  return quantities
}

export function cartInventoryLimitReached(variant: HttpTypes.StoreProductVariant | undefined, quantities: Record<string, number>) {
  return !!variant && !!variant.manage_inventory && !variant.allow_backorder &&
    (quantities[variant.id] ?? 0) >= (variant.inventory_quantity ?? 0)
}
