const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")
const qs = require("qs")

const source = fs.readFileSync(path.join(__dirname, "../src/lib/util/store-search-routing.ts"), "utf8")
const code = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS },
}).outputText
const loaded = { exports: {} }
new Function("require", "module", "exports", code)(
  () => ({ history: (options) => options }), loaded, loaded.exports
)
const { router, stateMapping } = loaded.exports.createStoreSearchRouting("product")
const createURL = (state) => router.createURL({
  qsModule: qs, routeState: stateMapping.stateToRoute(state),
  location: new URL("https://example.com/de-zh/store#products"),
})
const readURL = (url) => stateMapping.routeToState(router.parseURL({
  qsModule: qs, location: new URL(url),
}))

test("multiple refinements use short keys and literal comma separators", () => {
  const state = { product: { refinementList: {
    color_families: ["Silver", "Gold"], storage_options: ["128 GB", "256 GB"],
  } } }
  const url = createURL(state)
  assert.equal(url, "https://example.com/de-zh/store?color=Silver,Gold&storage=128%20GB,256%20GB#products")
  assert.deepEqual(readURL(url), state)
})

test("single selections and values containing commas or special characters round-trip", () => {
  for (const colors of [["Silver"], ["Red, White", "Gold"], ["黑色", "Blue & Green"], ["%2C", "100%"]]) {
    const state = { product: { refinementList: { color_families: colors } } }
    assert.deepEqual(readURL(createURL(state)), state)
  }
})

test("Next.js URL normalization preserves comma arrays and escaped values", () => {
  const state = { product: { query: "iPhone, Pro", refinementList: {
    color_families: ["Silver", "Gold", "Red, White", "%2C"],
  } } }
  const url = new URL(createURL(state))
  url.search = url.searchParams.toString()
  assert.deepEqual(readURL(url.href), state)
})

test("legacy indexed refinement URLs still restore selections", () => {
  const state = { product: { refinementList: { color_families: ["Silver", "Gold"] }, page: 2 } }
  assert.deepEqual(readURL(`https://example.com/de-en/store?${qs.stringify(state)}`), state)
})

test("search, pagination, sorting and price range survive URL restoration", () => {
  const state = { product: {
    query: "iPhone, Pro", page: 3, sortBy: "product/sort/min_price_eur:asc",
    range: { min_price_eur: "100:700" },
    refinementList: { category: ["iPhone 15"], condition_options: ["Excellent", "Good"] },
  } }
  assert.deepEqual(readURL(createURL(state)), state)
})

test("clear filters drops the query string and omits Configure state", () => {
  assert.equal(createURL({ product: { configure: { hitsPerPage: 12 } } }), "https://example.com/de-zh/store#products")
  assert.deepEqual(readURL("https://example.com/de-en/store"), { product: {} })
})

test("series links use the compact parameter and restore the series filter", () => {
  const seriesSource = fs.readFileSync(path.join(__dirname, "../src/lib/util/store-series-url.ts"), "utf8")
  const seriesModule = { exports: {} }
  new Function("module", "exports", ts.transpileModule(seriesSource, {
    compilerOptions: { module: ts.ModuleKind.CommonJS },
  }).outputText)(seriesModule, seriesModule.exports)
  assert.equal(seriesModule.exports.storeSeriesUrl(), "/store")
  const url = seriesModule.exports.storeSeriesUrl("iPhone 15")
  assert.equal(url, "/store?series=iPhone+15")
  assert.deepEqual(readURL(`https://example.com/de-en${url}`), {
    product: { refinementList: { category: ["iPhone 15"] } },
  })
})
