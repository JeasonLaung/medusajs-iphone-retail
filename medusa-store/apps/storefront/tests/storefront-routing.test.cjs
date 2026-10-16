const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")
const { NextRequest } = require("next/server")

function load(file, mocks = {}, globals = {}) {
  const source = fs.readFileSync(path.join(__dirname, "../src", file), "utf8")
  const code = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2021 },
  }).outputText
  const loaded = { exports: {} }
  new Function("require", "module", "exports", ...Object.keys(globals), code)(
    (name) => name in mocks ? mocks[name] : require(name), loaded, loaded.exports, ...Object.values(globals)
  )
  return loaded.exports
}

const routes = load("lib/util/storefront-route.ts")
const regions = [
  { id: "region_dk", currency_code: "dkk", countries: [{ iso_2: "dk" }] },
  { id: "region_de", currency_code: "eur", countries: [{ iso_2: "de" }] },
]

function middleware() {
  return load("middleware.ts", { "@lib/util/storefront-route": routes }, {
    process: { env: { NEXT_PUBLIC_MEDUSA_BACKEND_URL: "http://backend", NEXT_PUBLIC_DEFAULT_REGION: "dk" } },
    fetch: async () => ({ ok: true, json: async () => ({ regions }) }),
  }).middleware
}

test("explicit route language overrides a conflicting browser cookie", async () => {
  const response = await middleware()(new NextRequest("http://localhost/dk-en/store", {
    headers: { cookie: "_medusa_locale=zh-CN" },
  }))
  assert.equal(response.headers.get("location"), null)
  assert.equal(response.headers.get("x-middleware-request-x-storefront-locale"), "en")
  assert.equal(response.cookies.get("_medusa_locale").value, "en")
})

test("legacy country URLs preserve page and query when adding the language", async () => {
  const response = await middleware()(new NextRequest("http://localhost/de/store?sortBy=price_asc", {
    headers: { cookie: "_medusa_locale=zh-CN" },
  }))
  assert.equal(response.headers.get("location"), "http://localhost/de-zh/store?sortBy=price_asc")
})

test("the previous route cannot overwrite a locale-changing Server Action cookie", async () => {
  const response = await middleware()(new NextRequest("http://localhost/dk-zh/help", {
    method: "POST", headers: { cookie: "_medusa_locale=en", "next-action": "change-locale" },
  }))
  assert.equal(response.cookies.get("_medusa_locale"), undefined)
})

test("root and unsupported languages redirect to a canonical supported URL", async () => {
  const handle = middleware()
  assert.equal((await handle(new NextRequest("http://localhost/"))).headers.get("location"), "http://localhost/dk-en")
  assert.equal((await handle(new NextRequest("http://localhost/de-xx/help"))).headers.get("location"), "http://localhost/de-en/help")
})

test("language never changes the price region; country does", async () => {
  const { getRegion } = load("lib/data/regions.ts", {
    "@lib/config": { sdk: { client: { fetch: async () => ({ regions }) } } },
    "./cookies": { getCacheOptions: async () => ({}) },
    "@lib/util/storefront-route": routes,
  })
  assert.equal((await getRegion("dk-en")).id, "region_dk")
  assert.equal((await getRegion("dk-zh")).currency_code, "dkk")
  assert.equal((await getRegion("de-en")).id, "region_de")
  assert.equal((await getRegion("de-zh")).currency_code, "eur")
})
