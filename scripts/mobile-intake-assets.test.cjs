const assert = require("node:assert/strict")
const http = require("node:http")
const { before, after, test } = require("node:test")
const { gunzipSync } = require("node:zlib")
const { serveMobileIntakeAssets } = require("./mobile-intake-assets.cjs")
let server, port
before(async () => {
  server = http.createServer((request, response) => {
    if (!serveMobileIntakeAssets(request, response)) {
      response.writeHead(418, { "Content-Type": "text/plain" })
      response.end("original route")
    }
  })
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve))
  port = server.address().port
})
after(async () => {
  server.closeAllConnections()
  await new Promise(resolve => server.close(resolve))
})
function request(path, headers = {}, method = "GET") {
  return new Promise((resolve, reject) => {
    const call = http.request({ host: "127.0.0.1", port, path, method, headers }, response => {
      const chunks = []
      response.on("data", chunk => chunks.push(chunk))
      response.on("end", () => resolve({ status: response.statusCode, headers: response.headers, body: Buffer.concat(chunks) }))
      response.on("error", reject)
    })
    call.on("error", reject)
    call.end()
  })
}
test("only optimizes the standalone scanner prefix", async () => {
  for (const path of ["/app", "/admin/devices", "/mobile-intake", "/dk/store", "/static/other.js"]) {
    const response = await request(path)
    assert.equal(response.status, 418)
    assert.equal(response.body.toString(), "original route")
    assert.equal(response.headers["content-encoding"], undefined)
  }
})
test("serves HTML with one runtime request and no eager barcode library", async () => {
  const response = await request("/static/mobile-intake/index.html", { "accept-encoding": "gzip" })
  const html = gunzipSync(response.body).toString()
  assert.equal(response.status, 200)
  assert.equal((html.match(/<script\b/g) || []).length, 1)
  assert.match(html, /runtime\.js\?v=[a-f0-9]{20}/)
  assert(!html.includes('src="./vendor/zxing'))
  assert(!html.includes('<link rel="stylesheet"'))
  assert.equal(response.headers["cache-control"], "no-cache")
  assert(response.body.length < Buffer.byteLength(html) / 3)
})
test("versions the runtime and permits cache validation", async () => {
  const html = (await request("/static/mobile-intake/index.html")).body.toString()
  const runtimeUrl = html.match(/src="\.\/(runtime\.js\?v=[a-f0-9]+)"/)[1]
  const response = await request(`/static/mobile-intake/${runtimeUrl}`, { "accept-encoding": "gzip" })
  const source = gunzipSync(response.body).toString()
  assert.match(source, /Vue\.createApp/)
  assert.match(source, /const intakeLocales/)
  assert(!source.includes("class BrowserMultiFormatReader"))
  assert.match(response.headers["cache-control"], /immutable/)
  assert(response.body.length < Buffer.byteLength(source) / 2)
  const revalidated = await request(`/static/mobile-intake/${runtimeUrl}`, { "if-none-match": response.headers.etag })
  assert.equal(revalidated.status, 304)
  assert.equal(revalidated.body.length, 0)
})
test("supports HEAD and an explicit gzip opt-out", async () => {
  const head = await request("/static/mobile-intake/index.html", {}, "HEAD")
  assert.equal(head.status, 200)
  assert.equal(head.body.length, 0)
  const plain = await request("/static/mobile-intake/app.js", { "accept-encoding": "gzip;q=0,identity" })
  assert.equal(plain.headers["content-encoding"], undefined)
  assert.match(plain.body.toString(), /loadScannerLibrary/)
})
test("keeps the barcode library separately cached and rejects arbitrary files", async () => {
  const library = await request("/static/mobile-intake/vendor/zxing.min.js?v=0.1.5", { "accept-encoding": "gzip" })
  assert.equal(library.status, 200)
  assert.match(library.headers["cache-control"], /immutable/)
  for (const name of ["../.env", "%2e%2e/.env", "README.md", "unknown.js"]) {
    assert.equal((await request(`/static/mobile-intake/${name}`)).status, 404)
  }
  assert.equal((await request("/static/mobile-intake/index.html", {}, "POST")).status, 405)
})
