const https = require("node:https")
const http = require("node:http")
const net = require("node:net")
const fs = require("node:fs")
const path = require("node:path")
const tlsDirectory = path.resolve(__dirname, "../.device-camera-tls")
const port = Number(process.env.DEVICE_CAMERA_PORT || 9443)
let viteHmrPort = 0
const server = https.createServer({ key: fs.readFileSync(path.join(tlsDirectory, "server.key")), cert: fs.readFileSync(path.join(tlsDirectory, "server.crt")) }, (request, response) => {
  if (request.url === "/device-camera-ca.cer") {
    response.writeHead(200, { "Content-Type": "application/x-x509-ca-cert", "Content-Disposition": 'attachment; filename="device-camera-ca.cer"' })
    fs.createReadStream(path.join(tlsDirectory, "ca.cer")).pipe(response)
    return
  }
  const headers = { ...request.headers, host: "127.0.0.1:9001", "x-forwarded-proto": "https", "x-forwarded-host": request.headers.host }
  const isViteClient = request.url?.split("?")[0] === "/app/@vite/client"
  if (isViteClient) {
    headers["accept-encoding"] = "identity"
    delete headers["if-none-match"]
    delete headers["if-modified-since"]
  }
  // Keep same-origin checks aligned with the loopback upstream.
  if (headers.origin === `https://${request.headers.host}`) headers.origin = "http://127.0.0.1:9001"
  const upstream = http.request({ host: "127.0.0.1", port: 9001, method: request.method, path: request.url, headers }, (result) => {
    if (isViteClient && result.statusCode === 200) {
      const chunks = []
      result.on("data", (chunk) => chunks.push(chunk))
      result.on("end", () => {
        let body = Buffer.concat(chunks).toString("utf8")
        const match = body.match(/const hmrPort = (\d+);/)
        if (match) {
          viteHmrPort = Number(match[1])
          body = body.replace(/const hmrPort = \d+;/, `const hmrPort = ${port};`)
        }
        const responseHeaders = { ...result.headers, "cache-control": "no-store" }
        delete responseHeaders["content-length"]
        delete responseHeaders.etag
        response.writeHead(200, responseHeaders)
        response.end(body)
      })
      return
    }
    response.writeHead(result.statusCode || 502, result.headers)
    result.pipe(response)
  })
  upstream.on("error", () => { if (!response.headersSent) response.writeHead(502); response.end("Backend unavailable. Start start.bat first.") })
  request.pipe(upstream)
  request.on("aborted", () => upstream.destroy())
})
server.on("upgrade", (request, socket, head) => {
  const isViteHmr = String(request.headers["sec-websocket-protocol"] || "").includes("vite-hmr")
  const targetPort = isViteHmr && viteHmrPort ? viteHmrPort : 9001
  const upstream = net.connect(targetPort, "127.0.0.1", () => {
    const headers = { ...request.headers, host: "127.0.0.1:9001" }
    upstream.write(`${request.method} ${request.url} HTTP/${request.httpVersion}\r\n${Object.entries(headers).map(([key, value]) => `${key}: ${value}`).join("\r\n")}\r\n\r\n`)
    if (head.length) upstream.write(head)
    upstream.pipe(socket); socket.pipe(upstream)
  })
  upstream.on("error", () => socket.destroy())
  socket.on("error", () => upstream.destroy())
})
server.listen(port, "0.0.0.0", () => console.log(`Device camera HTTPS ready on port ${port}. Use the private LAN URL in logs/device-camera-url.txt.`))
