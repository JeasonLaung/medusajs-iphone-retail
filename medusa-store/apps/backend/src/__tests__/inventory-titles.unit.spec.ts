import { inventoryTitle, planInventoryTitles } from "../lib/inventory-titles"

describe("inventory titles", () => {
  it("combines model and variant without duplicating the model", () => {
    expect(inventoryTitle("iPhone 15", "128 GB / S / 85% / Yellow")).toBe("iPhone 15 / 128 GB / S / 85% / Yellow")
    expect(inventoryTitle("iPhone 15", "iPhone 15 / 128 GB")).toBe("iPhone 15 / 128 GB")
    expect(inventoryTitle("iPhone 15", "")).toBe("iPhone 15")
  })

  it("preserves custom titles and inventory shared with variants outside the selection", async () => {
    const graph = jest.fn().mockImplementation(({ entity }) => Promise.resolve({ data: entity === "product_variant" ? [
      { id: "v1", title: "128 GB", product: { title: "iPhone 15" }, inventory_items: [{ inventory_item_id: "i1", inventory: { title: "128 GB" } }] },
      { id: "v2", title: "256 GB", product: { title: "iPhone 16" }, inventory_items: [{ inventory_item_id: "i2", inventory: { title: "Custom inventory" } }] },
      { id: "v3", title: "512 GB", product: { title: "iPhone 16" }, inventory_items: [{ inventory_item_id: "i3", inventory: { title: "512 GB" } }] },
    ] : [
      { inventory_item_id: "i1", variant_id: "v1" },
      { inventory_item_id: "i3", variant_id: "v3" },
      { inventory_item_id: "i3", variant_id: "outside-selection" },
    ] }))
    const container = { resolve: () => ({ graph }) }
    expect(await planInventoryTitles(container as never, ["v1", "v2", "v3"])).toEqual([
      { id: "i1", title: "iPhone 15 / 128 GB", previous: "128 GB" },
    ])
  })
})
