const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")

function loadSource(file, mocks = {}) {
  const source = fs.readFileSync(path.join(__dirname, "../src", file), "utf8")
  const code = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2021 },
  }).outputText
  const loaded = { exports: {} }
  new Function("require", "module", "exports", code)(
    (name) => name in mocks ? mocks[name] : require(name), loaded, loaded.exports
  )
  return loaded.exports
}

const inventory = loadSource("lib/util/cart-inventory.ts")
const variant = { id: "variant_test", manage_inventory: true, allow_backorder: false, inventory_quantity: 1 }

test("cart quantities include every line for the same variant", () => {
  assert.deepEqual(inventory.cartVariantQuantities([
    { variant_id: variant.id, quantity: 1 },
    { variant_id: variant.id, quantity: 2 },
    { variant_id: null, quantity: 5 },
  ]), { [variant.id]: 3 })
})

test("the last available unit in the cart prevents another addition", () => {
  assert.equal(inventory.cartInventoryLimitReached(variant, {}), false)
  assert.equal(inventory.cartInventoryLimitReached(variant, { [variant.id]: 1 }), true)
  assert.equal(inventory.cartInventoryLimitReached({ ...variant, inventory_quantity: 2 }, { [variant.id]: 1 }), false)
  assert.equal(inventory.cartInventoryLimitReached({ ...variant, allow_backorder: true }, { [variant.id]: 1 }), false)
  assert.equal(inventory.cartInventoryLimitReached({ ...variant, manage_inventory: false }, { [variant.id]: 1 }), false)
})

function cartAction(createLineItem) {
  const tags = []
  const actions = loadSource("lib/data/cart.ts", {
    "@lib/config": { sdk: {
      client: { fetch: async () => ({ cart: { id: "cart_test", region_id: "region_test" } }) },
      store: { cart: { createLineItem } },
    } },
    "@lib/util/medusa-error": { default: (error) => { throw error } },
    "@lib/util/cart-inventory": inventory,
    "next/cache": { revalidateTag: (tag) => tags.push(tag) },
    "next/navigation": {},
    "./cookies": {
      getCartId: async () => "cart_test", getAuthHeaders: async () => ({}),
      getCacheOptions: async () => ({}), getCacheTag: async (tag) => tag,
    },
    "./regions": { getRegion: async () => ({ id: "region_test" }) },
    "./locale-actions": {}, "@lib/constants": {}, "@lib/util/storefront-route": {},
  })
  return { add: () => actions.addToCart({ variantId: variant.id, quantity: 1, countryCode: "dk-en" }), tags }
}

test("inventory rejection is returned as an expected result and refreshes stock and cart", async () => {
  const action = cartAction(async () => { throw new Error("Some variant does not have the required inventory") })
  assert.deepEqual(await action.add(), { ok: false, error: "inventory" })
  assert.deepEqual(action.tags, ["products", "carts"])
})

test("successful addition returns quantities confirmed by the backend", async () => {
  const action = cartAction(async () => ({ cart: { items: [{ variant_id: variant.id, quantity: 1 }] } }))
  assert.deepEqual(await action.add(), { ok: true, quantities: { [variant.id]: 1 } })
  assert.deepEqual(action.tags, ["carts", "fulfillment"])
})

test("unexpected API errors remain errors", async () => {
  const action = cartAction(async () => { throw new Error("Network unavailable") })
  await assert.rejects(action.add(), /Network unavailable/)
})
