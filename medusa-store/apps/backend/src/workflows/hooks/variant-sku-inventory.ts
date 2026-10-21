import { StepResponse } from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { createProductVariantsWorkflow, updateInventoryItemsWorkflow } from "@medusajs/medusa/core-flows"
import { planInventoryTitles } from "../../lib/inventory-titles"

// Core creation builds default inventory from the input SKU. Refetch the
// database-generated SKU before filling those otherwise empty inventory codes.
createProductVariantsWorkflow.hooks.productVariantsCreated(
  async ({ product_variants }, { container }) => {
    if (!product_variants.length) return new StepResponse([], [])
    const titles = await planInventoryTitles(container, product_variants.map(variant => variant.id))
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
    // Shared merchant inventory keeps its own SKU.
    const { data: links } = candidates.length ? await query.graph({
      entity: "product_variant_inventory_item",
      filters: { inventory_item_id: candidates.map((item) => item.id) },
      fields: ["inventory_item_id", "variant_id"],
      pagination: { take: 10000 },
    }) : { data: [] }
    const selected = candidates.filter((item) => links.filter((link) => link.inventory_item_id === item.id).length === 1)
    const updates = new Map<string, { id: string; sku?: string; title?: string }>()
    const previous = new Map<string, { id: string; sku?: string | null; title?: string | null }>()
    for (const item of selected) {
      updates.set(item.id, { id: item.id, sku: item.sku })
      previous.set(item.id, { id: item.id, sku: item.previous })
    }
    for (const item of titles) {
      updates.set(item.id, { ...updates.get(item.id), id: item.id, title: item.title })
      previous.set(item.id, { ...previous.get(item.id), id: item.id, title: item.previous })
    }
    if (updates.size) {
      await updateInventoryItemsWorkflow(container).run({ input: { updates: [...updates.values()] } })
    }
    return new StepResponse([...updates.keys()], [...previous.values()])
  },
  async (previous, { container }) => {
    if (previous?.length) await updateInventoryItemsWorkflow(container).run({ input: { updates: previous } })
  }
)
