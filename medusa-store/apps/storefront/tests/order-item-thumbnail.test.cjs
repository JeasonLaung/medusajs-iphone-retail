const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")

const source = fs.readFileSync(path.join(__dirname, "../src/lib/util/order-item-thumbnail.ts"), "utf8")
const code = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, esModuleInterop: true },
}).outputText
const loaded = { exports: {} }
new Function("require", "module", "exports", code)(
  () => require("../src/lib/util/iphone-thumbnails.json"), loaded, loaded.exports
)
const { getOrderItemThumbnail } = loaded.exports

test("orders prefer the selected SKU image over the saved product main image", () => {
  assert.equal(getOrderItemThumbnail({
    product_handle: "swappie-iphone-air",
    variant_title: "1024 GB / S / 85% / Sky Blue",
    thumbnail: "/swappie/swappie-iphone-air-space-black.png",
    variant: { thumbnail: "/static/iphone-products/iphone-air/sky-blue/01.webp" },
  }), "/static/iphone-products/iphone-air/sky-blue/01.webp")
})

test("historical orders recover known model/color images without a live SKU image", () => {
  for (const [color, slug] of [["Purple", "purple"], ["(PRODUCT)RED", "red"]]) {
    assert.equal(getOrderItemThumbnail({
      product_handle: "swappie-iphone-14",
      variant_title: `256 GB / A / 85% / ${color}`,
      thumbnail: "/old-midnight.png",
    }), `/iphone-thumbnails/iphone-14/${slug}.webp`)
  }
})

test("unknown colors and legacy storage-only orders preserve their saved image", () => {
  for (const title of ["128 GB", "256 GB / A / 85% / Unknown", null]) {
    assert.equal(getOrderItemThumbnail({
      product_handle: "swappie-iphone-14", variant_title: title, thumbnail: "/saved.png",
    }), "/saved.png")
  }
  assert.equal(getOrderItemThumbnail({ thumbnail: null }), null)
})
