const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")

function session(headers, fetchCustomer) {
  const source = fs.readFileSync(path.join(__dirname, "../src/lib/data/customer.ts"), "utf8")
  const code = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2021 },
  }).outputText
  const loaded = { exports: {} }
  const mocks = {
    "@lib/config": { sdk: { client: { fetch: fetchCustomer } } },
    "@lib/util/medusa-error": {},
    "@medusajs/js-sdk": {},
    "next/cache": {},
    "next/navigation": {},
    "./cookies": { getAuthHeaders: async () => headers },
  }
  new Function("require", "module", "exports", code)(
    (name) => {
      assert.ok(name in mocks, `Unexpected dependency: ${name}`)
      return mocks[name]
    }, loaded, loaded.exports
  )
  return loaded.exports.retrieveCustomer
}

test("a visitor without a login token does not request a customer", async () => {
  const retrieve = session({}, () => assert.fail("Anonymous customer request"))
  assert.equal(await retrieve(), null)
})

test("customer requests validate the current token without using persistent cache", async () => {
  const customer = { id: "cus_test" }
  const retrieve = session({ authorization: "Bearer valid" }, async (url, options) => {
    assert.equal(url, "/store/customers/me")
    assert.equal(options.headers.authorization, "Bearer valid")
    assert.equal(options.cache, "no-store")
    return { customer }
  })
  assert.equal(await retrieve(), customer)
})

test("a revoked session stops appearing logged in after a successful customer read", async () => {
  let valid = true
  const retrieve = session({ authorization: "Bearer revoked" }, async () => {
    if (!valid) throw Object.assign(new Error("Unauthorized"), { status: 401 })
    return { customer: { id: "cus_test" } }
  })
  assert.equal((await retrieve()).id, "cus_test")
  valid = false
  assert.equal(await retrieve(), null)
})

test("a backend failure does not leave the storefront showing a stale customer", async () => {
  const retrieve = session({ authorization: "Bearer token" }, async () => {
    throw new Error("Backend unavailable")
  })
  assert.equal(await retrieve(), null)
})
