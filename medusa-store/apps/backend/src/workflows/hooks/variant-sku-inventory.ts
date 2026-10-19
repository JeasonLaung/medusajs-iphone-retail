import { StepResponse } from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { createProductVariantsWorkflow, updateInventoryItemsWorkflow } from "@medusajs/medusa/core-flows"

// Core creation builds default inventory from the input SKU. Refetch the
// database-generated SKU before filling those otherwise empty inventory codes.
createProductVariantsWorkflow.hooks.productVariantsCreated(
  async ({ product_variants }, { container }) => {
    if (!product_variants.length) return new StepResponse([], [])
    const query = container.resolve(ContainerRegistrationKeys.QUERY)
    const { data: variants } = await query.graph({
      entity: "product_variant",
      filters: { id: product_variants.map((variant) => variant.id) },
      fields: ["id", "sku", "inventory_items.inventory_item_id", "inventory_items.inventory.sku"],
      pagination: { take: product_variants.length },
    })
    const candidates = variants.flatMap((variant) => {
      if (!/^ERP[1-9][0-9]{4,}$/.test(variant.sku ?? "") || variant.inventory_items?.length !== 1) return []
      const item = variant.inventory_items[0]!
      if (item.inventory?.sku?.trim()) return []
      return [{ id: item.inventory_item_id!, sku: variant.sku!, previous: item.inventory?.sku ?? null }]
    })
    if (!candidates.length) return new StepResponse([], [])
    // Shared merchant inventory keeps its own SKU.
    const { data: links } = await query.graph({
      entity: "product_variant_inventory_item",
      filters: { inventory_item_id: candidates.map((item) => item.id) },
      fields: ["inventory_item_id", "variant_id"],
      pagination: { take: 10000 },
    })
    const selected = candidates.filter((item) => links.filter((link) => link.inventory_item_id === item.id).length === 1)
    const previous = selected.map((item) => ({ id: item.id, sku: item.previous }))
    if (selected.length) {
      await updateInventoryItemsWorkflow(container).run({ input: { updates: selected.map(({ id, sku }) => ({ id, sku })) } })
    }
    return new StepResponse(selected.map((item) => item.id), previous)
  },
  async (previous, { container }) => {
    if (previous?.length) await updateInventoryItemsWorkflow(container).run({ input: { updates: previous } })
  }
)
