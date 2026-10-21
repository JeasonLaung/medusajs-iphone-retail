const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")

function load(file, mocks = {}) {
  const source = fs.readFileSync(path.join(__dirname, "../src", file), "utf8")
  const code = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2021,
      jsx: ts.JsxEmit.ReactJSX, esModuleInterop: true,
    },
  }).outputText
  const loaded = { exports: {} }
  new Function("require", "module", "exports", code)(
    (name) => name in mocks ? mocks[name] : require(name), loaded, loaded.exports
  )
  return loaded.exports
}

const details = load("lib/util/order-card-details.ts")
const withPayments = (...payments) => ({ payment_collections: [{ payments }] })

test("Tailwind generates all order status fills from the configured source directories", async () => {
  const postcss = require("postcss")
  const tailwind = require("tailwindcss")
  const config = require("../tailwind.config.js")
  const result = await postcss([tailwind({
    ...config,
    content: config.content.map((glob) => path.resolve(__dirname, "..", glob)),
  })]).process("@tailwind utilities;", { from: undefined })
  for (const key of ["fulfillmentShipped", "fulfillmentDelivered", "paymentAwaiting", "paymentRequiresAction", "fulfillmentCanceled"]) {
    const classes = details.getOrderStatusClasses(key).split(" ")
    for (const name of classes) {
      const color = name.match(/#[0-9A-F]{6}/)[0]
      assert.ok(result.css.includes(color), `Missing generated status color: ${name}`)
    }
  }
})

test("sale totals use the recorded original total, excluding credit-only reductions", () => {
  assert.equal(details.getOrderOriginalTotal({ total: 809, original_total: 899, discount_total: 90 }), 899)
  assert.equal(details.getOrderOriginalTotal({ total: 809, original_total: 899, discount_total: 0 }), null)
  assert.equal(details.getOrderOriginalTotal({ total: 809, original_total: 809, discount_total: 0 }), null)
  assert.equal(details.getOrderOriginalTotal({ total: 809, discount_total: 90 }), null)
})

test("one order tag reflects payment, shipment, refund and cancellation states", () => {
  const order = { status: "pending", payment_status: "captured", fulfillment_status: "not_fulfilled" }
  assert.equal(details.getOrderStatusKey(order), "paidAwaitingFulfillment")
  assert.equal(details.getOrderStatusKey({ ...order, fulfillment_status: "shipped" }), "fulfillmentShipped")
  assert.equal(details.getOrderStatusKey({ ...order, payment_status: "awaiting" }), "paymentAwaiting")
  assert.equal(details.getOrderStatusKey({ ...order, payment_status: "refunded", status: "completed" }), "paymentRefunded")
  assert.equal(details.getOrderStatusKey({ ...order, status: "canceled" }), "fulfillmentCanceled")
  assert.equal(details.getOrderStatusKey({ ...order, status: "completed" }), "completed")
})

test("unpaid or authorized orders do not use payment creation time as paid time", () => {
  assert.equal(details.getOrderPaidAt({}), null)
  assert.equal(details.getOrderPaidAt(withPayments({ created_at: "2026-10-21T02:00:00Z" })), null)
})

test("split payments use the latest actual capture across all collections", () => {
  assert.equal(details.getOrderPaidAt({ payment_collections: [
    { payments: [{ captured_at: "2026-10-21T01:02:03Z" }] },
    { payments: [{ captured_at: "2026-10-21T03:04:05Z" }, { captured_at: "invalid" }] },
  ] }), "2026-10-21T03:04:05.000Z")
})

test("a refunded payment keeps its original capture time", () => {
  assert.equal(details.getOrderPaidAt(withPayments({
    captured_at: "2026-10-21T01:02:03Z", refunded_amount: 809,
  })), "2026-10-21T01:02:03.000Z")
})

for (const lang of ["en", "zh"]) {
  test(`order cards render SKU, translated statuses and order date-time in ${lang}`, () => {
    const copy = require(`../src/modules/account/translations/${lang}.json`)
    const React = require("react")
    const { renderToStaticMarkup } = require("react-dom/server")
    const { default: OrderCard } = load("modules/account/components/order-card/index.tsx", {
      "@modules/account/use-account-copy": { useAccountCopy: () => ({
        t: (key) => { assert.equal(typeof copy[key], "string", key); return copy[key] },
        locale: lang === "zh" ? "zh-CN" : "en-GB",
      }) },
      "@modules/products/components/product-reviews/i18n": { getReviewCopy: () => ({ reviewProducts: "Review" }) },
      "@lib/util/order-number": { getOrderNumber: () => "2026102112345678901" },
      "@lib/util/order-card-details": details,
      "@modules/common/components/ui": { Button: ({ children }) => React.createElement("button", null, children) },
      "@modules/products/components/thumbnail": { default: () => null, __esModule: true },
      "@modules/common/components/localized-client-link": { default: ({ children }) => children, __esModule: true },
      "@lib/util/money": { convertToLocale: ({ amount }) => `€${amount.toFixed(2)}` },
    })
    const order = {
      ...withPayments({ captured_at: "2026-10-21T01:02:03Z" }),
      id: "order_test", status: "pending", payment_status: "captured", fulfillment_status: "not_fulfilled",
      created_at: "2026-10-20T00:00:00Z", total: 809, currency_code: "eur",
      items: [{ id: "item_test", title: "iPhone 16 Pro Max", quantity: 1, variant_sku: "IP16PM-256-DT-A" }],
    }
    const html = renderToStaticMarkup(React.createElement(OrderCard, { order }))
    assert.ok(html.includes("IP16PM-256-DT-A"))
    assert.ok(html.includes(copy.paidAwaitingFulfillment))
    assert.ok(html.indexOf('data-testid="order-status"') < html.indexOf('data-testid="order-display-id"'))
    assert.equal(html.includes('data-testid="order-payment-status"'), false)
    assert.ok(html.includes('dateTime="2026-10-20T00:00:00.000Z"'))
    assert.ok(html.includes("08:00:00"))
    assert.equal(html.includes('data-testid="order-paid-at"'), false)
    assert.ok(html.indexOf('data-testid="item-sku"') < html.indexOf('data-testid="order-amount"'))
    assert.equal(html.includes('data-testid="order-original-amount"'), false)
    const sale = renderToStaticMarkup(React.createElement(OrderCard, { order: {
      ...order, original_total: 899, discount_total: 90,
    } }))
    assert.ok(sale.includes('data-testid="order-original-amount"'))
    assert.ok(sale.includes("€899.00"))
    assert.ok(sale.includes("€809.00"))
    const unpaid = renderToStaticMarkup(React.createElement(OrderCard, { order: {
      ...order, payment_status: "awaiting", payment_collections: [],
      items: [{ ...order.items[0], variant_sku: null, variant: { sku: "FALLBACK-SKU" } }],
    } }))
    assert.ok(unpaid.includes(copy.paymentAwaiting))
    assert.ok(unpaid.includes("FALLBACK-SKU"))
  })
}
