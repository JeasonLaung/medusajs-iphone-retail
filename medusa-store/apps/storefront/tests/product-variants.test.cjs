const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")

function load(file, mocks = {}) {
  const source = fs.readFileSync(path.join(__dirname, "../src", file), "utf8")
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2021 } }).outputText
  const loaded = { exports: {} }
  new Function("require", "module", "exports", code)((name) => name in mocks ? mocks[name] : require(name), loaded, loaded.exports)
  return loaded.exports
}

const { availableOptionVariants, completeVariants, productHandleFromSlug, variantSlug, variantInStock, variantFromLocation, preferredOptionVariant } = load("lib/util/product-variants.ts")
const ids = ["storage", "condition", "battery", "color"]
const variant = (id, values, stock, extra = {}) => ({ id, manage_inventory: true, inventory_quantity: stock, options: values.map((value, i) => ({ option_id: ids[i], value })), ...extra })
const product = {
  options: ids.map((id) => ({ id, title: id === "storage" ? "Storage" : id[0].toUpperCase() + id.slice(1) })),
  variants: [
    variant("legacy", ["128 GB"], 100),
    variant("black", ["128 GB", "S", "85%", "Black"], 2),
    variant("blue", ["128 GB", "A", "100%", "Blue"], 1),
    variant("empty", ["256 GB", "S", "85%", "Black"], 0),
  ],
}
const selected = { storage: "128 GB", condition: "S", battery: "85%", color: "Black" }

test("a changed SKU URL takes precedence over the original server selection and query", () => {
  const variants = [
    { id: "original", metadata: { slug: "iphone-13-128gb-black-S85" } },
    { id: "changed", metadata: { slug: "iphone-13-128gb-blue-A100" } },
  ]
  assert.equal(variantFromLocation(variants, "/dk-en/products/iphone-13-128gb-blue-A100", "original", "original").id, "changed")
  assert.equal(variantFromLocation(variants, "/dk-en/products/iphone-13-128gb-black-s85", null, "changed").id, "original")
})

test("generic product URLs still support variant query links and initial selection", () => {
  assert.equal(variantFromLocation(product.variants, "/dk-en/products/swappie-iphone-13", "blue", "black").id, "blue")
  assert.equal(variantFromLocation(product.variants, "/dk-en/products/swappie-iphone-13", null, "black").id, "black")
  assert.equal(variantFromLocation(product.variants, "/dk-en/products/swappie-iphone-13", null), undefined)
})

test("legacy stock with missing attributes cannot make a full configuration available", () => {
  assert.equal(completeVariants(product).length, 3)
  assert.deepEqual(availableOptionVariants(product, selected, "storage", "256 GB"), [])
})
test("a stocked grade remains reachable when its battery and colour differ", () => {
  assert.deepEqual(availableOptionVariants(product, selected, "condition", "A").map((v) => v.id), ["blue"])
})
test("later options respect the preceding choices", () => {
  assert.deepEqual(availableOptionVariants(product, selected, "color", "Blue"), [])
  assert.deepEqual(availableOptionVariants(product, { ...selected, condition: "A", battery: "100%" }, "color", "Blue").map((v) => v.id), ["blue"])
})

test("option prices refer to the stocked SKU that selection will actually choose", () => {
  const extra = variant("closest", ["128 GB", "A", "85%", "Black"], 1)
  const choices = { ...product, variants: [...product.variants, extra] }
  assert.equal(preferredOptionVariant(choices, selected, "condition", "A").id, "closest")
  assert.equal(preferredOptionVariant(choices, selected, "storage", "256 GB"), undefined)
})

const { getAppleLaunchPrice } = load("lib/util/get-apple-launch-price.ts")
test("Apple launch prices follow capacity and currency, independently of refurbished grade", () => {
  const model = { ...product, title: "iPhone 13" }
  assert.equal(getAppleLaunchPrice(model, variant("512", ["512 GB", "A", "85%", "Blue"], 1), "eur"), 1189)
  assert.equal(getAppleLaunchPrice(model, product.variants[1], "eur"), 869)
  assert.equal(getAppleLaunchPrice(model, product.variants[1], "dkk"), null)
  assert.equal(getAppleLaunchPrice({ ...model, title: "iPhone 15" }, product.variants[1], "eur"), null)
  assert.equal(getAppleLaunchPrice(model, { ...product.variants[1], metadata: { apple_launch_price: { amount: 900, currency_code: "eur" } } }, "eur"), 900)
})
test("stock rules cover managed inventory, backorders and unmanaged inventory", () => {
  assert.equal(variantInStock(variant("a", [], 0)), false)
  assert.equal(variantInStock(variant("b", [], 0, { allow_backorder: true })), true)
  assert.equal(variantInStock(variant("c", [], 0, { manage_inventory: false })), true)
})
test("configuration URLs resolve models with multiword names and preserve S85/S100", () => {
  assert.equal(productHandleFromSlug("iphone-13-128gb-starlight-S85"), "swappie-iphone-13")
  assert.equal(productHandleFromSlug("iphone-15-pro-max-1024gb-natural-titanium-S100"), "swappie-iphone-15-pro-max")
  assert.equal(productHandleFromSlug("swappie-iphone-13"), "swappie-iphone-13")
  assert.equal(variantSlug({ metadata: { slug: "iphone-13-128gb-starlight-S85" } }), "iphone-13-128gb-starlight-S85")
  assert.equal(variantSlug({ metadata: { slug: "../invalid" } }), null)
})

const { getProductPrice } = load("lib/util/get-product-price.ts", {
  "./money": { convertToLocale: ({ amount }) => String(amount) },
  "./get-percentage-diff": { getPercentageDiff: () => "0" },
})
const priced = (id, amount) => ({ id, calculated_price: { calculated_amount: amount, original_amount: amount, currency_code: "eur", calculated_price: { price_list_type: "default" } } })
test("unpriced placeholder SKUs cannot erase the existing catalogue starting price", () => {
  const result = getProductPrice({ product: { id: "phone", variants: [priced("unset", null), priced("listed", 265)] } })
  assert.equal(result.cheapestPrice.calculated_price_number, 265)
})
test("a genuine zero price remains a valid price", () => {
  const result = getProductPrice({ product: { id: "phone", variants: [priced("free", 0)] }, variantId: "free" })
  assert.equal(result.variantPrice.calculated_price_number, 0)
})
