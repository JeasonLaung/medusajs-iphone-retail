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
  const mutations = []
  const actions = loadSource("lib/data/cart.ts", {
    "@lib/config": { sdk: {
      client: { fetch: async (url, options) => {
        requests.push({ url, ...options })
        return { cart: { id: cartId, region_id: regionId, completed_at: completedAt } }
      } },
      store: { cart: {
        createLineItem,
        updateLineItem: async (...args) => { mutations.push(["update", ...args]) },
        deleteLineItem: async (...args) => { mutations.push(["delete", ...args]) },
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
    update: () => actions.updateLineItem({ lineId: "line_test", quantity: 2 }),
    remove: () => actions.deleteLineItem("line_test"),
    setShipping: () => actions.setShippingMethod({ cartId: "cart_test", shippingMethodId: "shipping_test" }),
    tags, requests, created, saved, updated, redirects, shipping, mutations }
}

test("quantity updates and removal avoid returning an unused full cart and still invalidate totals", async () => {
  const action = cartAction(async () => ({}))
  await action.update()
  await action.remove()
  assert.deepEqual(action.mutations, [
    ["update", "cart_test", "line_test", { quantity: 2 }, { fields: "id" }, {}],
    ["delete", "cart_test", "line_test", { fields: "id" }, {}],
  ])
  assert.deepEqual(action.tags, ["carts", "fulfillment", "carts", "fulfillment"])
})

test("render reads share a cart within one request, while live mutation reads bypass memoization", async () => {
  const react = require(path.join(path.dirname(require.resolve("react/package.json")), "cjs/react.react-server.development.js"))
  const internal = react.__SERVER_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE
  const previous = internal.A
  let requests = 0
  const reads = loadSource("lib/data/cart.ts", {
    react,
    "@lib/config": { sdk: { client: { fetch: async () => ({ cart: { id: `cart_${++requests}` } }) } } },
    "@lib/util/medusa-error": {}, "@lib/util/cart-inventory": {},
    "next/cache": {}, "next/navigation": {},
    "./cookies": { getCartId: async () => "cart_test", getAuthHeaders: async () => ({}), getCacheOptions: async () => ({}) },
    "./regions": {}, "./locale-actions": {}, "@lib/constants": {}, "@lib/util/storefront-route": {},
  })
  const startRender = () => {
    const entries = new Map()
    internal.A = { getCacheForType: (factory) => {
      if (!entries.has(factory)) entries.set(factory, factory())
      return entries.get(factory)
    } }
  }
  try {
    startRender()
    const carts = await Promise.all([reads.retrieveCart(), reads.retrieveCart(), reads.retrieveCart()])
    assert.equal(requests, 1)
    assert.ok(carts.every(cart => cart === carts[0]))
    await reads.retrieveCart(undefined, "id", "no-store")
    await reads.retrieveCart(undefined, "id", "no-store")
    assert.equal(requests, 3)
    startRender()
    assert.equal((await reads.retrieveCart()).id, "cart_4")
  } finally { internal.A = previous }
})

test("cart stock lookup only serializes selected variants and keeps inventory fields", async () => {
  const calls = []
  const selected = { ...variant, options: [{ option_id: "color", value: "Pink" }], metadata: { slug: "iphone-17e-256gb-pink-A85" } }
  const actions = loadSource("lib/data/cart.ts", {
    "@lib/config": { sdk: { store: { product: { list: async (...args) => {
      calls.push(args)
      return { products: [{ variants: [selected, { id: "unused_variant" }] }] }
    } } } } },
    "@lib/util/medusa-error": {}, "@lib/util/cart-inventory": {},
    "next/cache": {}, "next/navigation": {}, "./cookies": { getAuthHeaders: async () => ({}) },
    "./regions": {}, "./locale-actions": {}, "@lib/constants": {}, "@lib/util/storefront-route": {},
  })
  assert.deepEqual(await actions.retrieveCartVariants(null), {})
  assert.equal(calls.length, 0)
  const result = await actions.retrieveCartVariants({ region_id: "region_test", items: [
    { product_id: "product_test", variant_id: variant.id }, { product_id: "product_test", variant_id: variant.id },
  ] })
  assert.deepEqual(result, { [variant.id]: selected })
  assert.deepEqual(calls[0][0].id, ["product_test"])
  assert.equal(calls[0][0].limit, 1)
  assert.match(calls[0][0].fields, /variants.inventory_quantity/)
  assert.match(calls[0][0].fields, /variants.manage_inventory/)
  assert.equal(calls[0][1]["cache-control"], "no-cache")
})

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

// Exercise the actual click handler without mounting unrelated product UI.
function productAddHandler(addToCart, countryCode) {
  const file = path.join(__dirname, "../src/modules/products/components/product-actions/index.tsx")
  const source = fs.readFileSync(file, "utf8")
  const tree = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
  let handler
  function visit(node) {
    if (ts.isVariableDeclaration(node) && node.name.getText(tree) === "handleAdd") handler = node.initializer.getText(tree)
    ts.forEachChild(node, visit)
  }
  visit(tree)
  assert.ok(handler, "ProductActions must expose its add handler")
  const code = ts.transpileModule(`module.exports = ${handler}`, {
    compilerOptions: { target: ts.ScriptTarget.ES2021, module: ts.ModuleKind.CommonJS },
  }).outputText
  const feedback = [], adding = [], routes = [], quantities = []
  const context = {
    canAdd: true, selectedVariant: variant, countryCode, addToCart,
    setIsAdding: (value) => adding.push(value), setFeedback: (value) => feedback.push(value),
    setQuantities: (value) => quantities.push(value), router: { push: (url) => routes.push(url) },
    unstable_rethrow: require("next/navigation").unstable_rethrow,
  }
  const loaded = { exports: {} }
  new Function("module", ...Object.keys(context), code)(loaded, ...Object.values(context))
  return { run: loaded.exports, feedback, adding, routes, quantities }
}

for (const countryCode of ["dk-en", "dk-zh"]) {
  test(`${countryCode}: successful server redirect never displays an add failure`, async () => {
    const { getRedirectError } = require("next/dist/client/components/redirect")
    const redirect = getRedirectError(`/${countryCode}/cart`, "push")
    const handler = productAddHandler(async () => { throw redirect }, countryCode)
    await assert.rejects(handler.run(), (error) => error === redirect)
    assert.deepEqual(handler.feedback, [null])
    assert.deepEqual(handler.routes, [])
    assert.deepEqual(handler.adding, [true, false])
  })

  test(`${countryCode}: real request and inventory failures still display feedback`, async () => {
    const request = productAddHandler(async () => { throw new Error("Network unavailable") }, countryCode)
    await request.run()
    assert.deepEqual(request.feedback, [null, "error"])
    const stock = productAddHandler(async () => ({ ok: false, error: "inventory" }), countryCode)
    await stock.run()
    assert.deepEqual(stock.feedback, [null, "inventory"])
    assert.deepEqual(stock.routes, [])
  })
}

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
