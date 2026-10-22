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

function cartAction(createLineItem, { completedAt = null, cartId = "cart_test", regionId = "region_test", locale = "zh-CN" } = {}) {
  const tags = []
  const requests = []
  const created = []
  const saved = []
  const updated = []
  const redirects = []
  const shipping = []
  const actions = loadSource("lib/data/cart.ts", {
    "@lib/config": { sdk: {
      client: { fetch: async (url, options) => {
        requests.push({ url, ...options })
        return { cart: { id: cartId, region_id: regionId, completed_at: completedAt } }
      } },
      store: { cart: {
        createLineItem,
        create: async (data) => {
          created.push(data)
          return { cart: { id: "cart_new", region_id: data.region_id } }
        },
        update: async (id, data) => { updated.push({ id, ...data }) },
        addShippingMethod: async (...args) => { shipping.push(args) },
      } },
    } },
    "@lib/util/medusa-error": { default: (error) => { throw error } },
    "@lib/util/cart-inventory": inventory,
    "next/cache": { revalidateTag: (tag) => tags.push(tag) },
    "next/navigation": { redirect: (url) => { redirects.push(url); throw new Error("NEXT_REDIRECT") } },
    "./cookies": {
      getCartId: async () => cartId, getAuthHeaders: async () => ({}),
      getCacheOptions: async () => ({}), getCacheTag: async (tag) => tag,
      setCartId: async (id) => { saved.push(id) },
    },
    "./regions": { getRegion: async () => ({ id: "region_test" }) },
    "./locale-actions": { getLocale: async () => locale }, "@lib/constants": {},
    "@lib/util/storefront-route": loadSource("lib/util/storefront-route.ts"),
  })
  return { add: (options = {}) => actions.addToCart({ variantId: variant.id, quantity: 1, countryCode: "dk-en", ...options }),
    setShipping: () => actions.setShippingMethod({ cartId: "cart_test", shippingMethodId: "shipping_test" }),
    tags, requests, created, saved, updated, redirects, shipping }
}

test("adding after checkout uses a new cart and bypasses stale cached completion state", async () => {
  let addedTo
  const action = cartAction(async (id) => {
    addedTo = id
    return { cart: { items: [{ variant_id: variant.id, quantity: 1 }] } }
  }, { completedAt: "2026-10-22T01:00:00Z", regionId: "region_previous" })
  assert.deepEqual(await action.add(), { ok: true, quantities: { [variant.id]: 1 } })
  assert.equal(addedTo, "cart_new")
  assert.equal(action.requests[0].cache, "no-store")
  assert.match(action.requests[0].query.fields, /completed_at/)
  assert.deepEqual(action.created, [{ region_id: "region_test", locale: "zh-CN" }])
  assert.deepEqual(action.saved, ["cart_new"])
  assert.deepEqual(action.updated, [])
})

test("an open cart keeps its ID and existing quantities", async () => {
  const action = cartAction(async (id) => {
    assert.equal(id, "cart_test")
    return { cart: { items: [{ variant_id: variant.id, quantity: 2 }] } }
  })
  assert.deepEqual(await action.add(), { ok: true, quantities: { [variant.id]: 2 } })
  assert.deepEqual(action.created, [])
  assert.deepEqual(action.saved, [])
})

test("an open cart still updates its region when the shopping country changes", async () => {
  const action = cartAction(async () => ({ cart: { items: [] } }), { regionId: "region_previous" })
  await action.add()
  assert.deepEqual(action.updated, [{ id: "cart_test", region_id: "region_test" }])
  assert.deepEqual(action.created, [])
})

test("a visitor without a cart still gets a new cart", async () => {
  const emptyAction = cartAction(async (id) => {
    assert.equal(id, "cart_new")
    return { cart: { items: [] } }
  }, { cartId: "" })
  await emptyAction.add()
  assert.deepEqual(emptyAction.requests, [])
  assert.deepEqual(emptyAction.saved, ["cart_new"])
})

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

test("product-page addition redirects after invalidation and requests only confirmed quantities", async () => {
  const action = cartAction(async (_id, _data, query) => {
    assert.equal(query.fields, "id,items.variant_id,items.quantity")
    return { cart: { items: [{ variant_id: variant.id, quantity: 1 }] } }
  })
  await assert.rejects(action.add({ redirectToCart: true, countryCode: "dk-zh" }), /^Error: NEXT_REDIRECT$/)
  assert.deepEqual(action.redirects, ["/dk-zh/cart"])
  assert.deepEqual(action.tags, ["carts", "fulfillment"])
})

test("inventory rejection stays on the product page even when redirect is requested", async () => {
  const action = cartAction(async () => { throw new Error("Insufficient inventory") })
  assert.deepEqual(await action.add({ redirectToCart: true }), { ok: false, error: "inventory" })
  assert.deepEqual(action.redirects, [])
})

test("direct cart navigation preserves English as well as Chinese", async () => {
  const action = cartAction(async () => ({ cart: { items: [] } }), { locale: "en" })
  await assert.rejects(action.add({ redirectToCart: true }), /^Error: NEXT_REDIRECT$/)
  assert.deepEqual(action.redirects, ["/dk-en/cart"])
})

test("shipping selection preserves cart invalidation without hydrating an unused full cart", async () => {
  const action = cartAction(async () => ({}))
  await action.setShipping()
  assert.deepEqual(action.shipping, [["cart_test", { option_id: "shipping_test" }, { fields: "id" }, {}]])
  assert.deepEqual(action.tags, ["carts"])
})
