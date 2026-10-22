const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")

function loadSource(file, mocks = {}) {
  const source = fs.readFileSync(path.join(__dirname, "../src", file), "utf8")
  const code = ts.transpileModule(source, {
    fileName: file,
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2021,
      jsx: ts.JsxEmit.ReactJSX,
    },
  }).outputText
  const loaded = { exports: {} }
  new Function("require", "module", "exports", code)(
    (name) => {
      if (name in mocks) return mocks[name]
      return require(name)
    },
    loaded,
    loaded.exports
  )
  return loaded.exports
}

const helpers = loadSource("lib/util/payment-session.ts")

test("production ignores manual sessions while development keeps them", () => {
  const previousEnvironment = process.env.NODE_ENV
  try {
    process.env.NODE_ENV = "production"
    const manual = { status: "authorized", provider_id: "pp_system_default" }
    const online = { status: "pending", provider_id: "pp_paypal_paypal" }
    assert.equal(helpers.isPaymentProviderAvailable(manual.provider_id), false)
    assert.equal(helpers.isPaymentProviderAvailable(online.provider_id), true)
    assert.equal(helpers.getActivePaymentSession({
      payment_collection: { payment_sessions: [manual, online] },
    }), online)
    assert.equal(helpers.getActivePaymentSession({
      payment_collection: { payment_sessions: [manual] },
    }), undefined)
    process.env.NODE_ENV = "development"
    assert.equal(helpers.isPaymentProviderAvailable(manual.provider_id), true)
    assert.equal(helpers.getActivePaymentSession({
      payment_collection: { payment_sessions: [manual] },
    }), manual)
  } finally {
    if (previousEnvironment === undefined) delete process.env.NODE_ENV
    else process.env.NODE_ENV = previousEnvironment
  }
})
test("checkout selects an actionable payment instead of a canceled first session", () => {
  const active = {
    id: "payses_current",
    status: "pending",
    provider_id: "pp_mollie-card_mollie",
  }
  assert.equal(
    helpers.getActivePaymentSession({
      payment_collection: {
        payment_sessions: [{ status: "canceled" }, active],
      },
    }),
    active
  )
})
test("an authorized payment takes precedence to finish an already paid order", () => {
  const paid = { id: "payses_paid", status: "authorized" }
  assert.equal(
    helpers.getActivePaymentSession({
      payment_collection: { payment_sessions: [{ status: "pending" }, paid] },
    }),
    paid
  )
})
test("Mollie checkout links must be HTTPS on Mollie's domain", () => {
  const data = (href) => ({ _links: { checkout: { href } } })
  assert.equal(
    helpers.getMollieCheckoutUrl(data("https://www.mollie.com/checkout/test")),
    "https://www.mollie.com/checkout/test"
  )
  for (const url of [
    "http://www.mollie.com/checkout",
    "https://mollie.com.evil.example/",
    "https://evil.example/",
    "javascript:alert(1)",
    "https://user:pass@mollie.com/",
  ]) {
    assert.equal(helpers.getMollieCheckoutUrl(data(url)), null)
  }
})

function returnHandler({
  cookie = "cart_current",
  session = { status: "pending", provider_id: "pp_mollie-hosted-checkout_mollie" },
  complete = async () => { throw new Error("Payment is pending") },
  retrieveError = false,
  cacheId,
  revalidateTag = () => {},
} = {}) {
  let fetches = 0, completions = 0
  const response = (data) => {
    const cleared = []
    return { ...data, cleared, cookies: { set: (key) => cleared.push(key) } }
  }
  const route = loadSource("app/api/mollie-return/route.ts", {
    "@lib/config": { sdk: {
      store: { cart: { complete: async (id) => { completions++; return complete(id) } } },
      client: { fetch: async () => {
        fetches++
        if (retrieveError) throw new Error("Backend unavailable")
        return { cart: {
          shipping_address: { country_code: "dk" },
          payment_collection: { payment_sessions: [session] },
        } }
      } },
    } },
    "next/cache": { revalidateTag },
    "@lib/util/storefront-route": loadSource("lib/util/storefront-route.ts"),
    "@lib/constants": { isMollie: (id) => id?.startsWith("pp_mollie-") },
    "next/server": { NextResponse: {
      redirect: (url) => response({ url: url.href }),
      json: (body, options) => response({ body, status: options?.status || 200 }),
    } },
  })
  const req = (value) => ({
    ...value,
    headers: value.headers || new Headers({ origin: "https://shop.example" }),
    cookies: { get: (key) => {
      const value = { _medusa_cart_id: cookie, _medusa_cache_id: cacheId }[key]
      return value ? { value } : undefined
    } },
  })
  return {
    GET: (value) => route.GET(req(value)), POST: (value) => route.POST(req(value)),
    counts: () => ({ fetches, completions }),
  }
}
const request = (id) => ({ nextUrl: new URL(
  `https://shop.example/api/mollie-return?cart_id=${id}&status=paid`
) })
test("Mollie returns to a dedicated result page before verification", async () => {
  const handler = returnHandler()
  assert.equal((await handler.GET(request("cart_current"))).url, "https://shop.example/payment-result?cart_id=cart_current")
  assert.deepEqual(handler.counts(), { fetches: 0, completions: 0 })
})
test("new return URLs preserve the checkout country", async () => {
  const handler = returnHandler()
  const result = await handler.GET({ nextUrl: new URL("https://shop.example/api/mollie-return?country_code=de&cart_id=cart_current") })
  assert.match(result.url, /de-en\/payment-result/)
})
test("a forged cart cannot complete another buyer's cart", async () => {
  const handler = returnHandler()
  assert.equal((await handler.POST(request("cart_other"))).body.status, "invalid")
  assert.deepEqual(handler.counts(), { fetches: 0, completions: 0 })
})
test("cross-origin verification cannot complete an order", async () => {
  const handler = returnHandler()
  const result = await handler.POST({ ...request("cart_current"), headers: new Headers({ origin: "https://evil.example" }) })
  assert.equal(result.status, 403)
  assert.deepEqual(handler.counts(), { fetches: 0, completions: 0 })
})
test("base return URL verifies only the browser's current cart", async () => {
  const handler = returnHandler({ complete: async (id) => {
    assert.equal(id, "cart_current")
    return { type: "order", order: { id: "order_test" } }
  } })
  const result = await handler.POST({ nextUrl: new URL("https://shop.example/api/mollie-return") })
  assert.equal(result.body.order_url, "/dk-en/order/order_test/confirmed")
})
test("expired cookies stay on the result page without completing an order", async () => {
  const handler = returnHandler({ cookie: null })
  assert.match((await handler.GET(request("cart_current"))).url, /payment-result/)
  assert.equal((await handler.POST(request("cart_current"))).body.status, "invalid")
  assert.deepEqual(handler.counts(), { fetches: 0, completions: 0 })
})
test("a forged status=paid stays pending until server confirmation", async () => {
  const handler = returnHandler()
  const result = await handler.POST(request("cart_current"))
  assert.deepEqual(result.body, { status: "pending" })
  assert.deepEqual(handler.counts(), { fetches: 2, completions: 1 })
  assert.deepEqual(result.cleared, [])
})
test("a Mollie return never completes manual payment", async () => {
  const handler = returnHandler({ session: { status: "pending", provider_id: "pp_system_default" } })
  assert.equal((await handler.POST(request("cart_current"))).body.status, "invalid")
  assert.equal(handler.counts().completions, 0)
})
test("canceled payment offers an explicit checkout link without redirecting", async () => {
  const handler = returnHandler({ session: { status: "canceled", provider_id: "pp_mollie-hosted-checkout_mollie" } })
  const result = await handler.POST(request("cart_current"))
  assert.deepEqual(result.body, { status: "failed", retry_url: "/dk-en/checkout?step=delivery" })
  assert.equal(result.url, undefined)
  assert.equal(handler.counts().completions, 0)
})
test("cancellation persisted during authorization is shown as failed", async () => {
  const session = { status: "pending", provider_id: "pp_mollie-hosted-checkout_mollie" }
  const handler = returnHandler({ session, complete: async () => {
    session.status = "canceled"
    throw new Error("Session was not authorized")
  } })
  assert.equal((await handler.POST(request("cart_current"))).body.status, "failed")
})
test("a verification outage never asks for another payment", async () => {
  const handler = returnHandler({ retrieveError: true })
  assert.deepEqual((await handler.POST(request("cart_current"))).body, { status: "error" })
  assert.equal(handler.counts().completions, 0)
})
test("confirmed orders supply the confirmation URL and clear the cart", async () => {
  const handler = returnHandler({ complete: async () => ({ type: "order", order: { id: "order_test" } }) })
  const result = await handler.POST(request("cart_current"))
  assert.deepEqual(result.body, { status: "confirmed", order_url: "/dk-en/order/order_test/confirmed" })
  assert.deepEqual(result.cleared, ["_medusa_cart_id"])
})
test("confirmed orders invalidate cart and order caches", async () => {
  const tags = []
  const handler = returnHandler({
    cacheId: "buyer-cache",
    revalidateTag: (tag) => tags.push(tag),
    complete: async () => ({ type: "order", order: { id: "order_test" } }),
  })
  const result = await handler.POST(request("cart_current"))
  assert.equal(result.body.status, "confirmed")
  assert.deepEqual(tags, ["carts-buyer-cache", "orders-buyer-cache"])
})
test("a cache invalidation failure cannot turn a confirmed order into pending", async () => {
  const handler = returnHandler({
    cacheId: "buyer-cache",
    revalidateTag: () => { throw new Error("Invariant: static generation store missing") },
    complete: async () => ({ type: "order", order: { id: "order_test" } }),
  })
  const result = await handler.POST(request("cart_current"))
  assert.deepEqual(result.body, { status: "confirmed", order_url: "/dk-en/order/order_test/confirmed" })
  assert.deepEqual(result.cleared, ["_medusa_cart_id"])
  assert.deepEqual(handler.counts(), { fetches: 1, completions: 1 })
})

function directMollie({ cartId = "cart_current", delivery = true, available = true, href = "https://www.mollie.com/checkout/test", status = "pending" } = {}) {
  let initiated = 0
  const cart = { id: cartId, region_id: "reg_test", total: 329 }
  const provider = "pp_mollie-hosted-checkout_mollie"
  const action = loadSource("lib/data/mollie.ts", {
    "@lib/config": { sdk: { store: { cart: { retrieve: async () => ({ cart }) } } } },
    "@lib/util/checkout-step": { checkoutStep: () => delivery ? "payment" : "delivery" },
    "@lib/util/payment-session": helpers,
    "@modules/checkout/i18n": { getCheckoutCopy: () => ({ paymentError: "Payment failed", completeDelivery: "Choose delivery", paymentUnavailable: "Unavailable" }) },
    "./cookies": { getCartId: async () => cartId, getAuthHeaders: async () => ({}) },
    "./locale-actions": { getLocale: async () => "en" },
    "./payment": { listCartPaymentMethods: async () => available ? [{ id: provider }] : [] },
    "./cart": { initiatePaymentSession: async (input, options) => {
      assert.equal(input, cart)
      assert.equal(options.provider_id, provider)
      initiated++
      return { payment_collection: { payment_sessions: [{ provider_id: provider, status, data: { _links: { checkout: { href } } } }] } }
    } },
  })
  return { start: action.startMolliePayment, initiated: () => initiated }
}
test("delivery continues directly to the Mollie hosted checkout", async () => {
  const action = directMollie()
  assert.equal(await action.start(), "https://www.mollie.com/checkout/test")
  assert.equal(action.initiated(), 1)
})
test("an already paid Mollie session resumes server-side order confirmation", async () => {
  const action = directMollie({ status: "authorized", href: null })
  assert.equal(await action.start(), "/api/mollie-return?cart_id=cart_current")
})
test("direct checkout cannot skip saved delivery or regional provider configuration", async () => {
  for (const options of [{ cartId: null }, { delivery: false }, { available: false }]) {
    const action = directMollie(options)
    await assert.rejects(action.start())
    assert.equal(action.initiated(), 0)
  }
})
test("direct checkout rejects invalid provider redirect URLs", async () => {
  const action = directMollie({ href: "https://evil.example/checkout" })
  await assert.rejects(action.start(), /Payment failed/)
})

test("payment result retries automatically and stops after confirmation", async () => {
  const originals = { fetch: global.fetch, setTimeout: global.setTimeout, clearTimeout: global.clearTimeout, window: global.window }
  const timers = []
  const states = []
  let effect, requests = 0, destination
  try {
    global.setTimeout = (callback) => { timers.push(callback); return timers.length }
    global.clearTimeout = () => {}
    global.window = { location: { replace: (url) => { destination = url } } }
    global.fetch = async () => ({ ok: true, json: async () => ++requests === 1
      ? { status: "pending" }
      : { status: "confirmed", order_url: "/dk-en/order/order_test/confirmed" } })
    const { default: Page } = loadSource("modules/checkout/components/payment-result/index.tsx", {
      react: { useState: (initial) => [initial, (value) => states.push(value)], useEffect: (callback) => { effect = callback } },
      "react/jsx-runtime": { jsx: () => null, jsxs: () => null },
      "next/link": {}, "@modules/common/components/brand-logo": {},
      "@modules/checkout/i18n": { getCheckoutCopy: () => ({}) }, "./styles.module.css": { default: {} },
    })
    Page({ countryCode: "dk-en", cartId: "cart_current" })
    const cleanup = effect()
    await timers.shift()()
    await new Promise((resolve) => setImmediate(resolve))
    assert.equal(states.at(-1).status, "pending")
    assert.equal(timers.length, 1)
    await timers.shift()()
    await new Promise((resolve) => setImmediate(resolve))
    assert.equal(destination, "/dk-en/order/order_test/confirmed")
    assert.equal(timers.length, 0)
    assert.equal(requests, 2)
    cleanup()
  } finally { Object.assign(global, originals) }
})
