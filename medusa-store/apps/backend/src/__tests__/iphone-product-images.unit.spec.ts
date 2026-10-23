jest.mock("@medusajs/medusa/core-flows", () => ({
  batchVariantImagesWorkflow: jest.fn(),
  updateProductVariantsWorkflow: jest.fn(),
}))

import { batchVariantImagesWorkflow, updateProductVariantsWorkflow } from "@medusajs/medusa/core-flows"
import syncImages from "../subscribers/iphone-product-images"
import { iphoneImageKey, variantImageKey } from "../lib/iphone-product-images"

const blue = { id: "blue", url: "http://localhost:9001/static/iphone-products/iphone-14/blue/01.webp" }
const red = { id: "red", url: "http://localhost:9001/static/iphone-products/iphone-14/product-red/01.webp" }
const otherModel = { id: "other", url: "http://localhost:9001/static/iphone-products/iphone-15/blue/01.webp" }
const links = jest.fn()
const thumbnails = jest.fn()

beforeEach(() => {
  jest.clearAllMocks()
  ;(batchVariantImagesWorkflow as unknown as jest.Mock).mockReturnValue({ run: links })
  ;(updateProductVariantsWorkflow as unknown as jest.Mock).mockReturnValue({ run: thumbnails })
})

async function run(color: string | null, images: typeof blue[], thumbnail: string | null = null) {
  const variant = {
    id: "variant", thumbnail, images,
    options: color ? [{ value: color, option: { title: "Color" } }] : [],
    product: { handle: "swappie-iphone-14", images: [otherModel, red, blue] },
  }
  const container = { resolve: () => ({ graph: async () => ({ data: [variant] }) }) }
  await syncImages({ event: { data: { id: "variant" } }, container } as unknown as Parameters<typeof syncImages>[0])
}

it("normalizes PRODUCT red and supports option IDs without inferring a missing color", () => {
  expect(iphoneImageKey(red.url)).toBe("iphone-14/red")
  expect(variantImageKey({ handle: "swappie-iphone-14", options: [{ id: "color", title: "颜色" }] }, { options: [{ option_id: "color", value: "Red" }] })).toBe("iphone-14/red")
  expect(variantImageKey({ handle: "swappie-iphone-14" }, { options: [] })).toBeNull()
})

it("maps a new SKU to its model and color, excluding the same color on another model", async () => {
  await run("Blue", [])
  expect(links).toHaveBeenCalledWith({ input: { variant_id: "variant", add: ["blue"], remove: [] } })
  expect(thumbnails).toHaveBeenCalledWith({ input: { product_variants: [{ id: "variant", thumbnail: blue.url }] } })
})

it("replaces the previous color association when a SKU color changes", async () => {
  await run("Red", [blue], blue.url)
  expect(links).toHaveBeenCalledWith({ input: { variant_id: "variant", add: ["red"], remove: ["blue"] } })
  expect(thumbnails).toHaveBeenCalledWith({ input: { product_variants: [{ id: "variant", thumbnail: red.url }] } })
})

it("does not write again for the thumbnail update event or unmatched colors", async () => {
  await run("Blue", [blue], blue.url)
  await run("Unknown", [red], red.url)
  await run(null, [red], red.url)
  expect(links).not.toHaveBeenCalled()
  expect(thumbnails).not.toHaveBeenCalled()
})
