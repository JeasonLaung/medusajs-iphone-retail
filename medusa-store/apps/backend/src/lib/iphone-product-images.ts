export function imageSlug(value: string) {
  return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "")
}

export function imageColor(value: string) {
  const slug = imageSlug(value)
  return slug === "product-red" ? "red" : slug
}

export function iphoneImageKey(url: string) {
  const match = url.match(/\/iphone-products\/([^/]+)\/([^/]+)\/[^/?]+(?:\?.*)?$/)
  return match ? `${match[1]}/${imageColor(match[2])}` : null
}

export function variantImageKey(product: { handle?: string | null; title?: string | null; options?: { id: string; title: string }[] | null }, variant: { options?: { option_id?: string | null; value: string; option?: { title?: string | null } | null }[] | null }) {
  const color = variant.options?.find((value) => /^(colou?r|颜色)$/i.test(value.option?.title ?? product.options?.find((option) => option.id === value.option_id)?.title ?? ""))?.value
  const model = imageSlug(product.handle?.replace(/^swappie-/, "") || product.title || "")
  return model && color ? `${model}/${imageColor(color)}` : null
}
