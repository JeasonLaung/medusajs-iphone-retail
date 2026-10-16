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

const { checkoutStep } = load("lib/util/checkout-step.ts", { "./payment-session": load("lib/util/payment-session.ts") })
const addressed = {
  email: "checkout@example.com",
  shipping_address: { first_name: "Checkout", last_name: "Test", address_1: "Test Street", city: "Copenhagen", postal_code: "2100", country_code: "dk" },
  billing_address: { first_name: "Checkout" },
  shipping_methods: [],
  total: 275,
}

test("direct payment and review links cannot skip incomplete contact information", () => {
  for (const field of ["first_name", "last_name", "address_1", "city", "postal_code", "country_code"]) {
    const cart = { ...addressed, shipping_address: { ...addressed.shipping_address, [field]: "" } }
    assert.equal(checkoutStep(cart, "payment"), "address")
    assert.equal(checkoutStep(cart, "review"), "address")
  }
  assert.equal(checkoutStep({ ...addressed, billing_address: null }, "payment"), "address")
})

test("delivery must be saved before payment, while earlier steps remain editable", () => {
  assert.equal(checkoutStep(addressed, "payment"), "delivery")
  const cart = { ...addressed, shipping_methods: [{ shipping_option_id: "standard" }] }
  assert.equal(checkoutStep(cart, "address"), "address")
  assert.equal(checkoutStep(cart, "delivery"), "delivery")
  assert.equal(checkoutStep(cart), "payment")
})

test("review requires a usable payment session or a fully paid gift card cart", () => {
  const cart = { ...addressed, shipping_methods: [{ shipping_option_id: "standard" }] }
  assert.equal(checkoutStep(cart, "review"), "payment")
  assert.equal(checkoutStep({ ...cart, payment_collection: { payment_sessions: [{ status: "canceled" }] } }, "review"), "payment")
  assert.equal(checkoutStep({ ...cart, payment_collection: { payment_sessions: [{ status: "pending" }] } }, "review"), "review")
  assert.equal(checkoutStep({ ...cart, total: 0 }, "review"), "payment")
  assert.equal(checkoutStep({ ...cart, total: 0, gift_cards: [{ id: "gift" }] }, "review"), "review")
})
