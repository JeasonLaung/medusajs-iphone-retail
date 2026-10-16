const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")

const source = fs.readFileSync(path.join(__dirname, "../src/lib/util/product-search-query.ts"), "utf8")
const loaded = { exports: {} }
new Function("module", "exports", ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS },
}).outputText)(loaded, loaded.exports)
const { normalizeProductSearchQuery: normalize } = loaded.exports
const { normalizeProductSearchRequest } = loaded.exports

test("abbreviations require all model words and preserve filters and pagination", () => {
  const query = {
    entity: "product",
    filters: { q: "i13p", category: "iPhone", storage_options: "256 GB" },
    pagination: { take: 50, skip: 0 },
    search_options: { match_strategy: "last", facets: ["model"] },
  }
  assert.deepEqual(normalizeProductSearchRequest(query), {
    ...query,
    filters: { ...query.filters, q: "iPhone 13 Pro" },
    search_options: { ...query.search_options, match_strategy: "all" },
  })
  assert.equal(query.filters.q, "i13p")
  for (const q of [undefined, "", "blue", "iPhone 14 Pro Max"]) {
    const ordinary = { ...query, filters: { q } }
    assert.equal(normalizeProductSearchRequest(ordinary), ordinary)
  }
})

test("iPhone model shortcuts match regardless of case", () => {
  for (const [input, expected] of [
    ["i14pm", "iPhone 14 Pro Max"],
    ["I14PM", "iPhone 14 Pro Max"],
    ["i14Pm", "iPhone 14 Pro Max"],
    ["i13p", "iPhone 13 Pro"],
    ["I13P", "iPhone 13 Pro"],
    ["i14", "iPhone 14"],
    ["i13mini", "iPhone 13 mini"],
    ["i14plus", "iPhone 14 Plus"],
    ["i16e", "iPhone 16e"],
    ["iphone14pro max", "iPhone 14 Pro Max"],
    ["i 14 pm", "iPhone 14 Pro Max"],
  ]) assert.equal(normalize(input), expected)
})

test("shortcuts preserve storage, colours and other search terms", () => {
  assert.equal(normalize("i14pm 256GB black"), "iPhone 14 Pro Max 256GB black")
  assert.equal(normalize("blue I13P"), "blue iPhone 13 Pro")
  assert.equal(normalize("i13p i14pm"), "iPhone 13 Pro iPhone 14 Pro Max")
})

test("ordinary queries and unrelated identifiers are preserved", () => {
  for (const query of ["", "iPhone 14 Pro Max", "iPhone SE", "blue", "128GB", "mini14", "sku-i14pmcase", "i14potato"]) {
    assert.equal(normalize(query), query)
  }
})
