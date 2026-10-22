const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")

function load(file, dependencies = {}) {
  const source = fs.readFileSync(path.join(__dirname, "../src", file), "utf8")
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
  const loaded = { exports: {} }
  new Function("require", "module", "exports", code)((name) => {
    assert.ok(name in dependencies, `Unexpected dependency: ${name}`)
    return dependencies[name]
  }, loaded, loaded.exports)
  return loaded.exports
}
const errors = load("lib/util/after-sales-error.ts")
const en = require("../src/modules/account/translations/en.json")
const zh = require("../src/modules/account/translations/zh.json")

test("business errors have specific copy in Chinese and English", () => {
  for (const [status, message, expected] of [
    [400, "Select the shipped devices for this order item", "afterSalesShippedDevicesRequired"],
    [400, "Choose valid order items and quantities", "afterSalesInvalidItems"],
    [400, "This order item has no registered devices", "afterSalesDevicesChanged"],
    [400, "This order cannot receive an after-sales request", "afterSalesUnavailable"],
    [400, "An after-sales request is already open for this order", "openRequest"],
    [401, "Unauthorized", "afterSalesSessionExpired"],
    [404, "Order not found", "afterSalesOrderNotLinked"],
    [500, "Internal database details", "submitRequestError"],
  ]) {
    const key = errors.getAfterSalesErrorKey({ status, message })
    assert.equal(key, expected)
    assert.ok(en[key])
    assert.ok(zh[key])
  }
})

test("submitting forwards an unshipped return without adding device IDs", async () => {
  const body = { type: "return", reason: "Please return this order", items: [{ item_id: "i1", quantity: 1 }] }
  const { submitAfterSales } = load("lib/data/after-sales.ts", {
    "@lib/config": { sdk: { client: { fetch: async (url, options) => {
      assert.equal(url, "/store/orders/o1/after-sales")
      assert.deepEqual(options.body, body)
      assert.equal(options.method, "POST")
      assert.equal(options.headers.authorization, "Bearer test")
      return { request: { id: "r1" } }
    } } } },
    "./cookies": { getAuthHeaders: async () => ({ authorization: "Bearer test" }) },
    "@lib/util/after-sales-error": errors,
  })
  assert.equal((await submitAfterSales("o1", body)).request.id, "r1")
})

test("the server action preserves the specific device validation error for the UI", async () => {
  const { submitAfterSales } = load("lib/data/after-sales.ts", {
    "@lib/config": { sdk: { client: { fetch: async () => {
      throw Object.assign(new Error("Select the shipped devices for this order item"), { status: 400 })
    } } } },
    "./cookies": { getAuthHeaders: async () => ({ authorization: "Bearer test" }) },
    "@lib/util/after-sales-error": errors,
  })
  const result = await submitAfterSales("o1", {})
  assert.equal(result.request, null)
  assert.equal(result.errorKey, "afterSalesShippedDevicesRequired")
})
