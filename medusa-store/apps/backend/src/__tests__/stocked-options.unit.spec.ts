import { loadStockedVariantIds, stockedStorageOptions } from "../search/helpers/stocked-options"
import { getVariantAvailability } from "@medusajs/framework/utils"
import { resolveProductIds } from "../search/helpers/resolve-product-ids"

jest.mock("@medusajs/framework/utils", () => ({ getVariantAvailability: jest.fn() }))

const option = (value: string) => ({ value, option: { title: "Storage" } })

describe("product card capacity stock", () => {
  it("keeps only stocked capacities, independent of SKU presence", () => {
    const product = { id: "p", variants: [
      { id: "128", options: [option("128 GB")] },
      { id: "256", sku: "existing-sku", options: [option("256 GB")] },
      { id: "128-blue", options: [option("128 GB")] },
    ] }
    expect(stockedStorageOptions(product, new Set(["128", "128-blue"]))).toEqual(["128 GB"])
    expect(stockedStorageOptions(product, new Set())).toEqual([])
    expect(stockedStorageOptions(product, new Set(["256"]))).toEqual(["256 GB"])
  })

  it("requires positive sellable stock in the product's sales channel", async () => {
    const query = {}
    const context = { container: { query } } as any
    jest.mocked(getVariantAvailability).mockResolvedValue({
      positive: { availability: 1, sales_channel_id: "web" },
      reserved: { availability: 0, sales_channel_id: "web" },
      negative: { availability: -1, sales_channel_id: "web" },
      missing: { availability: null, sales_channel_id: "web" },
    })
    expect(await loadStockedVariantIds([{
      id: "p", sales_channels: [{ id: "web" }],
      variants: ["positive", "reserved", "negative", "missing"].map((id) => ({ id })),
    }], context)).toEqual(new Set(["positive"]))
    expect(getVariantAvailability).toHaveBeenCalledWith(query, {
      variant_ids: ["positive", "reserved", "negative", "missing"], sales_channel_id: "web",
    })
  })

  it.each(["inventory-level.updated", "inventory-level.deleted", "reservation-item.created", "reservation-item.deleted"])("refreshes affected products after %s", async (name) => {
    const graph = jest.fn()
      .mockResolvedValueOnce({ data: [{ inventory_item_id: "item" }] })
      .mockResolvedValueOnce({ data: [{ variant: { product_id: "p" } }, { variant: { product_id: "p" } }] })
    expect(await resolveProductIds({ name, data: { id: "row" } }, { container: { query: { graph } } } as any)).toEqual(["p"])
    expect(graph.mock.calls[0][0]).toEqual({
      entity: name.startsWith("inventory-level") ? "inventory_level" : "reservation",
      fields: ["inventory_item_id"], filters: { id: ["row"] }, withDeleted: name.endsWith(".deleted"),
    })
  })
})
