const fs = require("node:fs")
const path = require("node:path")
const { createHash } = require("node:crypto")
const { gzipSync } = require("node:zlib")

const prefix = "/static/mobile-intake/"
const root = path.resolve(__dirname, "../medusa-store/apps/backend/static/mobile-intake")
const sourceFiles = ["index.html", "style.css", "locales.js", "app.js", "vendor/vue.global.prod.js", "vendor/vant.min.js", "vendor/vant.css", "vendor/zxing.min.js", "vendor/zxing-wasm-reader-3.1.4.js", "vendor/zxing_reader-3.1.4.wasm"]
const scripts = ["vendor/vue.global.prod.js", "vendor/vant.min.js", "locales.js", "app.js"]
let snapshot

function asset(body, type) {
  const raw = Buffer.from(body)
  const hash = createHash("sha256").update(raw).digest("hex").slice(0, 20)
  return { raw, compressed: gzipSync(raw), hash, type, etag: `W/"${hash}"` }
}

function readAssets() {
  const revision = sourceFiles.map(file => {
    const stat = fs.statSync(path.join(root, file))
    return `${stat.mtimeMs}:${stat.size}`
  }).join(";")
  if (snapshot?.revision === revision) return snapshot.assets
  const sources = Object.fromEntries(sourceFiles.map(file => [file, fs.readFileSync(path.join(root, file), file.endsWith(".wasm") ? undefined : "utf8")]))
  const assets = {}
  for (const file of sourceFiles) {
    const type = file.endsWith(".css") ? "text/css" : file.endsWith(".html") ? "text/html" : "application/javascript"
    assets[file] = asset(sources[file], file.endsWith(".wasm") ? "application/wasm" : `${type}; charset=utf-8`)
  }
  // Vue and Vant still run as ordinary scripts, in their original dependency order.
  // One runtime request replaces four parallel requests across the FRP tunnel.
  assets["runtime.js"] = asset(scripts.map(file => sources[file]).join("\n;\n"), "application/javascript; charset=utf-8")
  let html = sources["index.html"]
    .replace(/\s*<link\b[^>]*href="\.\/(?:vendor\/vant\.css|style\.css)(?:\?[^" ]*)?"[^>]*>/g, "")
    .replace(/\s*<script\b[^>]*src="\.\/(?:vendor\/vue\.global\.prod\.js|vendor\/vant\.min\.js|vendor\/zxing\.min\.js|locales\.js|app\.js)(?:\?[^" ]*)?"[^>]*><\/script>/g, "")
  html = html.replace("</head>", `<style>${sources["vendor/vant.css"]}\n${sources["style.css"]}</style>\n<script defer src="./runtime.js?v=${assets["runtime.js"].hash}"></script>\n</head>`)
  assets["index.html"] = asset(html, "text/html; charset=utf-8")
  snapshot = { revision, assets }
  return assets
}

function acceptsGzip(value = "") {
  return value.split(",").some(item => {
    const [name, ...parameters] = item.trim().split(";")
    return name === "gzip" && !parameters.some(parameter => /^\s*q\s*=\s*0(?:\.0*)?\s*$/.test(parameter))
  })
}

function serveMobileIntakeAssets(request, response) {
  const pathname = request.url.split("?")[0]
  if (!pathname.startsWith(prefix)) return false
  if (!["GET", "HEAD"].includes(request.method)) {
    response.writeHead(405, { Allow: "GET, HEAD" })
    response.end()
    return true
  }
  const name = pathname.slice(prefix.length) || "index.html"
  if (![...sourceFiles, "runtime.js"].includes(name)) {
    response.writeHead(404)
    response.end()
    return true
  }
  try {
    const current = readAssets()[name]
    const query = new URL(request.url, "http://localhost").searchParams
    const versioned = query.get("v") === current.hash || (name === "vendor/zxing.min.js" && query.get("v") === "0.1.5")
    const headers = {
      "Content-Type": current.type,
      "X-Content-Type-Options": "nosniff",
      "Cache-Control": versioned ? "public, max-age=86400, immutable" : "no-cache",
      "ETag": current.etag,
      "Vary": "Accept-Encoding",
    }
    if (String(request.headers["if-none-match"] || "").split(/,\s*/).includes(current.etag)) {
      response.writeHead(304, headers)
      response.end()
      return true
    }
    const compressed = acceptsGzip(request.headers["accept-encoding"])
    const body = compressed ? current.compressed : current.raw
    if (compressed) headers["Content-Encoding"] = "gzip"
    headers["Content-Length"] = body.length
    response.writeHead(200, headers)
    response.end(request.method === "HEAD" ? undefined : body)
  } catch {
    response.writeHead(503, { "Cache-Control": "no-store", "Content-Type": "text/plain; charset=utf-8" })
    response.end("Mobile intake assets unavailable")
  }
  return true
}

module.exports = { serveMobileIntakeAssets }
