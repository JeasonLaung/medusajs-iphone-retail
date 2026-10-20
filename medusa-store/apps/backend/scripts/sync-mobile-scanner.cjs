const fs = require("node:fs")
const path = require("node:path")

// Keep the standalone mobile scanner on the same decoder as the admin.
const wasm = require.resolve("zxing-wasm/reader/zxing_reader.wasm")
const dist = path.resolve(path.dirname(wasm), "..")
const destination = path.resolve(__dirname, "../static/mobile-intake/vendor")
fs.copyFileSync(path.join(dist, "iife/reader/index.js"), path.join(destination, "zxing-wasm-reader-3.1.4.js"))
fs.copyFileSync(wasm, path.join(destination, "zxing_reader-3.1.4.wasm"))
fs.copyFileSync(path.resolve(dist, "../LICENSE"), path.join(destination, "zxing-wasm-LICENSE.txt"))
