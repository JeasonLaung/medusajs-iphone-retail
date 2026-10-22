import "server-only"

import { sdk } from "@lib/config"

// Reuse the catalogue until buyback eligibility has its own configuration.
export type SellModel = { name: string; handle: string; capacities: string[] }

export async function getSellModels(): Promise<SellModel[]> {
  const models = new Map<string, SellModel>()
  let offset = 0
  while (true) {
    const { products, count } = await sdk.store.product.list(
      { fields: "id,title,handle,*options,*options.values", limit: 100, offset },
      { cache: "no-store" }
    )
    for (const product of products) {
      const title = product.title.trim()
      if (!/^iPhone\b/i.test(title) || !product.handle) continue
      const capacities = (product.options ?? [])
        .filter((option) => /storage|capacity|memory|容量/i.test(option.title))
        .flatMap((option) => option.values ?? [])
        .map(({ value }) => value.replace(/\s+/g, "").toUpperCase())
        .filter((value) => /^\d+(GB|TB)$/.test(value))
      models.set(title, { name: title, handle: product.handle.replace(/^swappie-/, ""),
        capacities: Array.from(new Set(capacities)).sort((a, b) =>
          parseInt(a) * (a.endsWith("TB") ? 1024 : 1) - parseInt(b) * (b.endsWith("TB") ? 1024 : 1)) })
    }
    offset += products.length
    if (!products.length || offset >= count) break
  }
  const rank = (name: string) => {
    if (name === "iPhone Air") return 173
    const generation = Number(name.match(/^iPhone (\d+)/)?.[1] ?? 0)
    const variant = name.includes("Pro Max") ? 5 : name.includes("Pro") ? 4 : name.includes("Plus") ? 3 : name.includes("mini") ? 0 : /\de$/.test(name) ? 1 : 2
    return generation * 10 + variant
  }
  return Array.from(models.values()).sort((a, b) => rank(b.name) - rank(a.name) || b.name.localeCompare(a.name, "en", { numeric: true }))
}
