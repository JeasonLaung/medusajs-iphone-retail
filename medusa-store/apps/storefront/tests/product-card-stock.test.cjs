const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")

function load(relativePath) {
  const filename = path.resolve(__dirname, relativePath)
  const code = ts.transpileModule(fs.readFileSync(filename, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS },
  }).outputText
  const module = { exports: {} }
  new Function("require", "module", "exports", code)(
    (name) => load(`../src/lib/util/${name}.ts`), module, module.exports
  )
  return module.exports
}
const { productCardStock, refreshProductCardStock } = load("../src/lib/util/product-card-stock.ts")
const variant = (id, storage, color, stock) => ({
  id, inventory_quantity: stock,
  options: [{ option_id: "storage", value: storage }, { option_id: "color", value: color }],
})
const product = {
  id: "p", options: [{ id: "storage", title: "Storage" }, { id: "color", title: "Colour" }],
  variants: [variant("a", "128 GB", "Black", 1), variant("b", "256 GB", "White", 0)],
}

test("only positive sellable quantities enable capacities and colors", () => {
  assert.deepEqual(productCardStock({ ...product, variants: [
    ...product.variants, variant("c", "512 GB", "Blue", -1), variant("d", "1024 GB", "Gold", undefined),
  ] }), { stocked_storage_options: ["128 GB"], stocked_color_options: ["Black"] })
})

test("another stocked SKU keeps a capacity available and incomplete variants do not", () => {
  assert.deepEqual(productCardStock({ ...product, variants: [
    ...product.variants, variant("c", "256 GB", "Black", 2),
    { id: "legacy", inventory_quantity: 10, options: [{ option_id: "storage", value: "512 GB" }] },
  ] }), { stocked_storage_options: ["128 GB", "256 GB"], stocked_color_options: ["Black"] })
})

test("missing products have no stock", () => {
  assert.deepEqual(productCardStock(), { stocked_storage_options: [], stocked_color_options: [] })
})

test("live quantities replace stale index stock while preserving hit order and search metadata", async () => {
  const hit = { objectID: "p", title: "iPhone", stocked_storage_options: ["256 GB"], sku_color_options: ["White"] }
  const results = [{ hits: [hit, { objectID: "gone", stocked_storage_options: ["128 GB"] }], nbHits: 17, facets: { storage_options: { "128 GB": 3 } } }, { hits: [hit] }]
  const actual = await refreshProductCardStock(results, async (ids) => {
    assert.deepEqual(ids, ["p", "gone"])
    return [product]
  })
  assert.deepEqual(actual[0].hits[0].stocked_storage_options, ["128 GB"])
  assert.deepEqual(actual[0].hits[0].stocked_color_options, ["Black"])
  assert.deepEqual(actual[0].hits[1].stocked_storage_options, [])
  assert.equal(actual[0].nbHits, 17)
  assert.deepEqual(actual[0].facets, results[0].facets)
  assert.deepEqual(hit.stocked_storage_options, ["256 GB"])
})

test("stock lookup errors do not silently restore stale availability", async () => {
  await assert.rejects(refreshProductCardStock([{ hits: [{ objectID: "p" }] }], async () => {
    throw new Error("Inventory unavailable")
  }), /Inventory unavailable/)
  assert.deepEqual(await refreshProductCardStock([{ hits: [] }], () => assert.fail("no request needed")), [{ hits: [] }])
})
