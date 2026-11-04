const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")
const source = fs.readFileSync(path.join(__dirname, "../src/middleware.ts"), "utf8")
const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
const loaded = { exports: {} }
new Function("require", "module", "exports", code)((name) => {
  if (name === "next/server") return { NextResponse: { redirect: (url, status) => ({ url, status }) } }
  if (name === "@lib/util/storefront-route") return {}
  throw new Error(`Unexpected import: ${name}`)
}, loaded, loaded.exports)

test("saved model links redirect in both languages without losing variant queries", async () => {
  for (const locale of ["dk-en", "dk-zh"]) {
    const nextUrl = new URL(`https://shop.example/${locale}/products/swappie-iphone-15-pro-max?v_id=variant_123`)
    nextUrl.clone = () => new URL(nextUrl)
    const response = await loaded.exports.middleware({ nextUrl })
    assert.equal(response.status, 308)
    assert.equal(response.url.href, `https://shop.example/${locale}/products/iphone-15-pro-max?v_id=variant_123`)
  }
})

test("historical product images resolve to the identical renamed asset", async () => {
  const nextUrl = new URL("https://shop.example/swappie/swappie-iphone-17e-soft-pink.png?v=old")
  nextUrl.clone = () => new URL(nextUrl)
  const response = await loaded.exports.middleware({ nextUrl })
  assert.equal(response.status, 308)
  assert.equal(response.url.href, "https://shop.example/product-images/iphone-17e-soft-pink.png?v=old")
  assert(fs.existsSync(path.join(__dirname, "../public", response.url.pathname)))
})
