import type { ExecArgs } from "@medusajs/framework/types"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import {
  createProductCategoriesWorkflow,
  updateProductCategoriesWorkflow,
  updateProductsWorkflow,
} from "@medusajs/medusa/core-flows"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

// Keep the catalog's parent category on every product: the store filters by it.
export default async function setupIphoneSeries({ container }: Pick<ExecArgs, "container">) {
  const manifest = JSON.parse(
    readFileSync(
      resolve(process.cwd(), "../../../imports/swappie-iphone-catalog.json"),
      "utf8"
    )
  ) as { products: { model: string; slug: string }[] }
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data: categories } = await query.graph({
    entity: "product_category",
    fields: ["id", "name", "handle", "parent_category_id"],
  })
  const parent = categories.find((category) => category.handle === "iphone")
    ?? categories.find((category) => category.name === "iPhone" && !category.parent_category_id)
    ?? categories.find((category) => category.name === "Swappie iPhones")
  if (!parent) throw new Error("Import the iPhone catalog first")

  const { data: products } = await query.graph({
    entity: "product",
    fields: ["id", "handle", "description", "categories.id"],
    filters: { handle: manifest.products.map((item) => `swappie-${item.slug}`) },
  })
  const groups = new Map<string, { name: string; productIds: string[] }>()
  // Validate the complete mapping before changing any data.
  for (const item of manifest.products) {
    const match = item.model.match(/^iPhone (\d+|SE|Air)(?:\b|e\b)/)
    if (!match) throw new Error(`Unrecognized iPhone family: ${item.model}`)
    const family = match[1]
    const handle = `iphone-${family.toLowerCase()}-series`
    const product = products.find((entry) => entry.handle === `swappie-${item.slug}`)
    if (!product) throw new Error(`Missing catalog product: ${item.model}`)
    const existing = categories.find((category) => category.handle === handle)
    if (existing && existing.parent_category_id !== parent.id) {
      throw new Error(`Category ${handle} already exists under a different parent`)
    }
    const group = groups.get(handle) ?? { name: `iPhone ${family}`, productIds: [] }
    group.productIds.push(product.id)
    groups.set(handle, group)
  }

  const renamed = parent.name !== "iPhone" || parent.handle !== "iphone"
  if (renamed) {
    await updateProductCategoriesWorkflow(container).run({
      input: { selector: { id: parent.id }, update: { name: "iPhone", handle: "iphone" } },
    })
  }

  let updated = 0
  for (const [handle, group] of groups) {
    let categoryId = categories.find((category) => category.handle === handle)?.id
    if (!categoryId) {
      const { result } = await createProductCategoriesWorkflow(container).run({
        input: {
          product_categories: [{
            name: group.name,
            handle,
            parent_category_id: parent.id,
            is_active: true,
            is_internal: false,
          }],
        },
      })
      categoryId = result[0].id
    }
    const changes = group.productIds.flatMap((id) => {
      const product = products.find((entry) => entry.id === id)!
      const currentIds = (product.categories ?? []).flatMap((category) => category ? [category.id] : [])
      const categoryIds = [...new Set([...currentIds, parent.id, categoryId!])]
      const description = product.description?.replace(
        "Reference catalog entry from the supplied Swappie category snapshot.",
        "Reference catalog entry from the supplied iPhone catalog snapshot."
      )
      return !renamed && categoryIds.length === currentIds.length && description === product.description
        ? []
        : [{ id, category_ids: categoryIds, description }]
    })
    if (changes.length) {
      await updateProductsWorkflow(container).run({ input: { products: changes } })
      updated += changes.length
    }
    console.log(`${group.name}: ${group.productIds.length} products`)
  }
  console.log(`iPhone series ready: ${groups.size} categories, ${updated} products updated`)
}
