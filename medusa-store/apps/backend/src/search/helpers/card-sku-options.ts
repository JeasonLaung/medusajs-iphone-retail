type Variant = {
  sku?: string | null
  options?: ({ value?: string | null; option?: { title?: string | null } | null } | null)[] | null
} | null

export function cardSkuOptions(variants: Variant[] | null | undefined, storages: unknown) {
  const skuStorage = new Set<string>()
  const skuColors = new Set<string>()
  for (const variant of variants ?? []) {
    if (!variant?.sku?.trim()) continue
    for (const option of variant.options ?? []) {
      const title = option?.option?.title?.trim() ?? ""
      const value = option?.value?.trim()
      if (!value) continue
      if (/^(storage|capacity|容量)$/i.test(title)) skuStorage.add(value)
      if (/^(colou?r|颜色)$/i.test(title)) skuColors.add(value)
    }
  }
  const catalogStorage = Array.isArray(storages)
    ? storages.filter((value): value is string => typeof value === "string" && Boolean(value.trim())).map((value) => value.trim())
    : []
  return {
    model_storage_options: Array.from(new Set([...catalogStorage, ...skuStorage])),
    sku_storage_options: Array.from(skuStorage),
    sku_color_options: Array.from(skuColors),
  }
}
