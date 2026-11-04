const http = require("node:http")
const domain = "frp.xeappliance.com"
const port = 18080
const backendPath = /^\/(admin|auth|store|health|mobile-intake|hooks|api|mail)(\/|$)/
function isApiRequest(request) {
  try {
    const pathname = new URL(decodeURIComponent(request.url), "http://localhost").pathname
    return backendPath.test(pathname)
  } catch { return false }
}
function headersFor(request, targetPort) {
  const headers = { ...request.headers, "x-forwarded-host": request.headers.host, "x-forwarded-proto": "https" }
  if (targetPort === 9001) {
    headers.host = "127.0.0.1:9001"
    if (headers.origin === `https://${domain}`) headers.origin = "http://127.0.0.1:9001"
  }
  return headers
}
const server = http.createServer((request, response) => {
  if (!isApiRequest(request)) {
    response.writeHead(404, { "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "no-store" })
    response.end("API only. Open storefront at http://localhost:8000 or Admin at http://localhost:9001/app.")
    return
  }
  const targetPort = 9001
  const headers = headersFor(request, targetPort)
  const upstream = http.request({ host: "127.0.0.1", port: targetPort, method: request.method, path: request.url, headers }, result => {
    const responseHeaders = { ...result.headers }
    if (responseHeaders.location) {
      responseHeaders.location = responseHeaders.location.replace(/^https?:\/\/(?:localhost|127\.0\.0\.1)(?::\d+)?/, `https://${domain}`)
    }
    response.writeHead(result.statusCode || 502, responseHeaders)
    result.pipe(response)
  })
  upstream.on("error", () => {
    if (!response.headersSent) response.writeHead(502)
    response.end("Local service unavailable. Run start.bat.")
  })
  request.on("aborted", () => upstream.destroy())
  request.pipe(upstream)
})
server.on("upgrade", (_request, socket) => {
  socket.end("HTTP/1.1 404 Not Found\r\nConnection: close\r\n\r\n")
})
server.listen(port, "127.0.0.1", () => console.log(`frp gateway ready on 127.0.0.1:${port}`))
