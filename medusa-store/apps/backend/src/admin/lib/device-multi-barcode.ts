import { prepareZXingModule, readBarcodes } from "zxing-wasm/reader"
import wasmUrl from "zxing-wasm/reader/zxing_reader.wasm?url"

// Serve the decoder with the admin app; no uploaded labels or CDN requests.
prepareZXingModule({ overrides: { locateFile: (path: string, prefix: string) => path.endsWith(".wasm") ? wasmUrl : prefix + path } })

export async function decodeDeviceBarcodes(image: File | ImageData) {
  const results = await readBarcodes(image, { tryHarder: true, maxNumberOfSymbols: 16 })
  results.sort((a, b) => a.position.topLeft.y - b.position.topLeft.y || a.position.topLeft.x - b.position.topLeft.x)
  return [...new Set(results.map((result) => result.text.trim()).filter(Boolean))]
}
