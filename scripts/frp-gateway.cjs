const http = require("node:http")
const net = require("node:net")
const domain = "frp.xeappliance.com"
const port = 18080
let viteHmrPort = 0
const backendPath = /^\/(app|admin|auth|store|health|static|uploads)(\/|\?|$)/
function target(request) { return backendPath.test(request.url) ? 9001 : 8000 }
function headersFor(request, targetPort) {
  const headers = { ...request.headers, "x-forwarded-host": request.headers.host, "x-forwarded-proto": "https" }
  if (targetPort === 9001) {
    headers.host = "127.0.0.1:9001"
    if (headers.origin === `https://${domain}`) headers.origin = "http://127.0.0.1:9001"
  }
  return headers
}
const server = http.createServer((request, response) => {
  const targetPort = target(request)
  const headers = headersFor(request, targetPort)
  const viteClient = request.url?.split("?")[0] === "/app/@vite/client"
  if (viteClient) {
    headers["accept-encoding"] = "identity"
    delete headers["if-none-match"]
    delete headers["if-modified-since"]
  }
  const upstream = http.request({ host: "127.0.0.1", port: targetPort, method: request.method, path: request.url, headers }, result => {
    const responseHeaders = { ...result.headers }
    if (responseHeaders.location) {
      responseHeaders.location = responseHeaders.location.replace(/^https?:\/\/(?:localhost|127\.0\.0\.1)(?::\d+)?/, `https://${domain}`)
    }
    if (viteClient && result.statusCode === 200) {
      const chunks = []
      result.on("data", chunk => chunks.push(chunk))
      result.on("end", () => {
        let body = Buffer.concat(chunks).toString("utf8")
        const match = body.match(/const hmrPort = (\d+);/)
        if (match) {
          viteHmrPort = Number(match[1])
          body = body.replace(/const hmrPort = \d+;/, "const hmrPort = 443;")
        }
        delete responseHeaders["content-length"]
        delete responseHeaders.etag
        responseHeaders["cache-control"] = "no-store"
        response.writeHead(200, responseHeaders)
        response.end(body)
      })
    } else {
      response.writeHead(result.statusCode || 502, responseHeaders)
      result.pipe(response)
    }
  })
  upstream.on("error", () => {
    if (!response.headersSent) response.writeHead(502)
    response.end("Local service unavailable. Run start.bat.")
  })
  request.on("aborted", () => upstream.destroy())
  request.pipe(upstream)
})
server.on("upgrade", (request, socket, head) => {
  const isVite = String(request.headers["sec-websocket-protocol"] || "").includes("vite-hmr")
  const targetPort = isVite && viteHmrPort ? viteHmrPort : target(request)
  const upstream = net.connect(targetPort, "127.0.0.1", () => {
    const headers = headersFor(request, isVite ? 9001 : targetPort)
    upstream.write(`${request.method} ${request.url} HTTP/${request.httpVersion}\r\n${Object.entries(headers).map(([key, value]) => `${key}: ${value}`).join("\r\n")}\r\n\r\n`)
    if (head.length) upstream.write(head)
    upstream.pipe(socket)
    socket.pipe(upstream)
  })
  upstream.on("error", () => socket.destroy())
  socket.on("error", () => upstream.destroy())
  socket.on("close", () => upstream.destroy())
})
server.listen(port, "127.0.0.1", () => console.log(`frp gateway ready on 127.0.0.1:${port}`))
