import type { HttpTypes } from "@medusajs/types"

export const variantOptions = (variant: HttpTypes.StoreProductVariant) =>
  Object.fromEntries((variant.options ?? []).filter((option) => option.option_id).map((option) => [option.option_id!, option.value]))

export const variantInStock = (variant?: HttpTypes.StoreProductVariant) => !!variant &&
  (!variant.manage_inventory || variant.allow_backorder || (variant.inventory_quantity ?? 0) > 0)

export function purchaseOptions(product: HttpTypes.StoreProduct) {
  const order = ["storage", "condition", "battery", "color", "colour"]
  return [...(product.options ?? [])].sort((a, b) => {
    const rank = (title: string | undefined) => {
      const index = order.indexOf((title ?? "").toLowerCase())
      return index < 0 ? order.length : index
    }
    return rank(a.title) - rank(b.title)
  })
}

export function completeVariants(product: HttpTypes.StoreProduct) {
  return (product.variants ?? []).filter((variant) => {
    const values = variantOptions(variant)
    return (product.options ?? []).every((option) => values[option.id])
  })
}

// Earlier selections constrain later groups. A change can reset later choices
// to a stocked combination, so a different battery/colour is never unreachable.
export function availableOptionVariants(product: HttpTypes.StoreProduct, selected: Record<string, string>, optionId: string, value: string) {
  const options = purchaseOptions(product)
  const preceding = options.slice(0, options.findIndex((option) => option.id === optionId))
  return completeVariants(product).filter((variant) => {
    const values = variantOptions(variant)
    return variantInStock(variant) && values[optionId] === value && preceding.every((option) => !selected[option.id] || selected[option.id] === values[option.id])
  })
}

export function preferredOptionVariant(product: HttpTypes.StoreProduct, selected: Record<string, string>, optionId: string, value: string) {
  const score = (variant: HttpTypes.StoreProductVariant) => Object.entries(variantOptions(variant)).filter(([key, val]) => selected[key] === val).length
  return availableOptionVariants(product, selected, optionId, value)
    .reduce<HttpTypes.StoreProductVariant | undefined>((best, variant) => !best || score(variant) > score(best) ? variant : best, undefined)
}

export function productHandleFromSlug(slug: string) {
  const match = slug.match(/^(iphone-.+?)-\d+(?:gb|tb)-[a-z0-9-]+-[sa](?:85|100)$/i)
  return match ? `swappie-${match[1].toLowerCase()}` : slug
}

export function variantSlug(variant?: HttpTypes.StoreProductVariant) {
  const slug = variant?.metadata?.slug
  return typeof slug === "string" && /^iphone-[a-z0-9-]+-[SA](?:85|100)$/.test(slug) ? slug : null
}

export function variantFromLocation(
  variants: HttpTypes.StoreProductVariant[],
  pathname: string,
  variantId: string | null,
  initialVariantId?: string
) {
  const slug = pathname.split("/").pop()?.toLowerCase()
  return variants.find((variant) => variantSlug(variant)?.toLowerCase() === slug)
    ?? variants.find((variant) => variant.id === variantId)
    ?? variants.find((variant) => variant.id === initialVariantId)
}
