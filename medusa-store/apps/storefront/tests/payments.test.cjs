const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")

function loadSource(file, mocks = {}) {
  const source = fs.readFileSync(path.join(__dirname, "../src", file), "utf8")
  const code = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2021,
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
  session = {
    status: "pending",
    provider_id: "pp_mollie-hosted-checkout_mollie",
  },
  complete = async () => {
    throw new Error("Payment is pending")
  },
} = {}) {
  let fetches = 0,
    completions = 0
  const redirectError = { redirect: true }
  const route = loadSource("app/api/mollie-return/route.ts", {
    "@lib/config": {
      sdk: {
        client: {
          fetch: async () => {
            fetches++
            return {
              cart: {
                shipping_address: { country_code: "dk" },
                payment_collection: { payment_sessions: [session] },
              },
            }
          },
        },
      },
    },
    "@lib/data/cart": {
      placeOrder: async (id) => {
        completions++
        return complete(id, redirectError)
      },
    },
    "@lib/data/cookies": {
      getAuthHeaders: async () => ({}),
      getCartId: async () => cookie,
    },
    "@lib/constants": { isMollie: (id) => id?.startsWith("pp_mollie-") },
    "next/navigation": {
      unstable_rethrow: (error) => {
        if (error === redirectError) throw error
      },
    },
    "next/server": { NextResponse: { redirect: (url) => ({ url: url.href }) } },
  })
  return {
    GET: route.GET,
    counts: () => ({ fetches, completions }),
    redirectError,
  }
}
const request = (id) => ({
  nextUrl: new URL(
    `https://shop.example/api/mollie-return?cart_id=${id}&status=paid`
  ),
})
test("a forged return cannot replace or complete another buyer's cart", async () => {
  const handler = returnHandler()
  const result = await handler.GET(request("cart_other"))
  assert.match(result.url, /payment_status=invalid/)
  assert.deepEqual(handler.counts(), { fetches: 0, completions: 0 })
})
test("a browser status=paid does not bypass server-side payment confirmation", async () => {
  const handler = returnHandler()
  const result = await handler.GET(request("cart_current"))
  assert.match(
    result.url,
    /\/dk\/checkout\?step=payment&payment_status=pending/
  )
  assert.deepEqual(handler.counts(), { fetches: 1, completions: 1 })
})
test("a Mollie return never completes a cart using manual payment", async () => {
  const handler = returnHandler({
    session: { status: "pending", provider_id: "pp_system_default" },
  })
  assert.match(
    (await handler.GET(request("cart_current"))).url,
    /payment_status=failed/
  )
  assert.equal(handler.counts().completions, 0)
})
test("canceled payments return to the payment step with a retry message", async () => {
  const handler = returnHandler({
    complete: async () => {
      throw new Error("Payment is not authorized: current status is canceled")
    },
  })
  assert.match(
    (await handler.GET(request("cart_current"))).url,
    /payment_status=failed/
  )
})
test("successful order confirmation redirects are preserved", async () => {
  const handler = returnHandler({
    complete: async (_id, redirectError) => {
      throw redirectError
    },
  })
  await assert.rejects(
    handler.GET(request("cart_current")),
    (error) => error === handler.redirectError
  )
})
