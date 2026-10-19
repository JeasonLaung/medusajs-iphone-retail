import type { ExecArgs } from "@medusajs/framework/types"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { createProductsWorkflow, createProductVariantsWorkflow, deleteProductsWorkflow } from "@medusajs/medusa/core-flows"
import { strict as assert } from "node:assert"

// Disposable draft product verifies both normal admin creation workflows.
export default async function checkSkuNumbering({ container }: ExecArgs) {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const stamp = String(Date.now())
  let productId: string | undefined
  try {
    const { result: products } = await createProductsWorkflow(container).run({ input: { products: [{
      title: `SKU numbering check ${stamp}`,
      handle: `sku-numbering-check-${stamp}`,
      status: "draft",
      options: [{ title: "Check", values: ["omitted", "empty", "custom", "later"] }],
      variants: [
        { title: "Omitted SKU", options: { Check: "omitted" }, manage_inventory: true },
        { title: "Empty SKU", sku: "", options: { Check: "empty" }, manage_inventory: true },
        { title: "Custom SKU", sku: `SKU-CHECK-${stamp}`, options: { Check: "custom" }, manage_inventory: true },
      ],
    }] } })
    productId = products[0].id
    const { result: later } = await createProductVariantsWorkflow(container).run({ input: { product_variants: [{
      product_id: productId, title: "Later variant", options: { Check: "later" }, manage_inventory: true,
    }] } })
    const { data: variants } = await query.graph({
      entity: "product_variant",
      filters: { product_id: productId },
      fields: ["id", "title", "sku", "inventory_items.inventory.sku"],
      pagination: { take: 10 },
    })
    assert.equal(variants.length, 4)
    assert.equal(new Set(variants.map((variant) => variant.sku)).size, 4)
    for (const variant of variants) {
      if (variant.title === "Custom SKU") assert.equal(variant.sku, `SKU-CHECK-${stamp}`)
      else assert.match(variant.sku!, /^ERP[1-9][0-9]{4,}$/)
      assert.equal(variant.inventory_items?.length, 1)
      assert.equal(variant.inventory_items![0]!.inventory?.sku, variant.sku, `Inventory SKU mismatch: ${variant.title}`)
    }
    assert(variants.some((variant) => variant.id === later[0].id))
    // The standard admin endpoints refetch the persisted variant after creation.
    console.log("VERIFIED: omitted and empty SKUs auto-generated through product and variant creation; custom SKU preserved; refetched variant and inventory SKUs match.")
  } finally {
    if (productId) await deleteProductsWorkflow(container).run({ input: { ids: [productId] } })
  }
}
