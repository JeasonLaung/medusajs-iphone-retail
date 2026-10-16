import { cardSkuOptions } from "../search/helpers/card-sku-options"

const option = (title: string, value: string) => ({ value, option: { title } })

describe("product card SKU availability", () => {
  it("keeps model capacities even when no corresponding SKU exists", () => {
    expect(cardSkuOptions([
      { sku: "iphone-11-64-black", options: [option("Storage", "64 GB"), option("Color", "Black")] },
      { sku: "iphone-11-128-white", options: [option("Storage", "128 GB"), option("Color", "White")] },
    ], ["64 GB", "128 GB", "256 GB"])).toEqual({
      model_storage_options: ["64 GB", "128 GB", "256 GB"],
      sku_storage_options: ["64 GB", "128 GB"],
      sku_color_options: ["Black", "White"],
    })
  })

  it("does not treat unassigned option definitions as an existing SKU", () => {
    expect(cardSkuOptions([
      { sku: " ", options: [option("Storage", "256 GB"), option("Color", "Pink")] },
      { options: [option("Storage", "512 GB")] },
      null,
    ], ["256 GB"])).toEqual({
      model_storage_options: ["256 GB"], sku_storage_options: [], sku_color_options: [],
    })
  })

  it("deduplicates actual SKU options and supports colour/capacity aliases", () => {
    expect(cardSkuOptions([
      { sku: "a", options: [option(" Capacity ", " 128 GB "), option("Colour", " Blue ")] },
      { sku: "b", options: [option("容量", "128 GB"), option("颜色", "Blue"), null] },
    ], null)).toEqual({
      model_storage_options: ["128 GB"], sku_storage_options: ["128 GB"], sku_color_options: ["Blue"],
    })
  })
})
