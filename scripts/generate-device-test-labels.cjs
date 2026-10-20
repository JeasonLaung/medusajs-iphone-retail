const assert = require("node:assert/strict")
const fs = require("node:fs/promises")
const path = require("node:path")
const { createRequire } = require("node:module")

// Reuse check-device-multi-scan.ts's writer without booting Medusa or accessing its database.
const root = path.resolve(__dirname, "..")
const requireBackend = createRequire(path.join(root, "medusa-store/apps/backend/package.json"))
const writer = requireBackend("zxing-wasm/writer")
const reader = requireBackend("zxing-wasm/reader")
const sharp = require(process.env.SHARP_MODULE_PATH || "sharp")
const output = path.resolve(process.argv[2] || path.join(root, "logs/device-test-labels"))
const escapeXml = (value) => String(value).replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&apos;" })[char])

function imeiFor(index) {
  // Synthetic TAC 00000000; length and Luhn check digit are suitable for scanner tests.
  const body = `00000000${String(index).padStart(6, "0")}`
  const sum = [...body].reduce((total, digit, position) => {
    const value = Number(digit) * (position % 2 ? 2 : 1)
    return total + Math.floor(value / 10) + value % 10
  }, 0)
  return body + (10 - sum % 10) % 10
}

async function main() {
  await fs.mkdir(output, { recursive: true })
  for (const [library, kind] of [[writer, "writer"], [reader, "reader"]]) {
    const wasmBinary = new Uint8Array(await fs.readFile(requireBackend.resolve(`zxing-wasm/${kind}/zxing_${kind}.wasm`)))
    library.prepareZXingModule({ overrides: { wasmBinary } })
  }
  const { products } = JSON.parse(await fs.readFile(path.join(root, "imports/swappie-iphone-catalog.json"), "utf8"))
  const labels = []
  for (const [index, product] of products.entries()) {
    const label = {
      model: product.model,
      storage_gb: product.selected_storage_gb,
      color: product.available_colors[0],
      serial: `TEST${String(index + 1).padStart(8, "0")}`,
      imei: imeiFor(index + 1),
      eid: `89049032000000000000${String(index + 1).padStart(12, "0")}`,
      file: product.slug,
    }
    const parts = [
      '<svg xmlns="http://www.w3.org/2000/svg" width="1120" height="860" viewBox="0 0 1120 860">',
      '<rect width="1120" height="860" fill="white"/>',
      `<g fill="black" font-family="Arial, sans-serif"><text x="48" y="60" font-size="32" font-weight="bold">${escapeXml(label.model)}</text>`,
      `<text x="48" y="100" font-size="24">${label.storage_gb}GB / ${escapeXml(label.color)}</text>`,
      '<text x="48" y="138" font-size="20">TEST ONLY / 仅供测试 — Synthetic identifiers / 模拟号码</text>',
    ]
    for (const [row, [name, value]] of [["S/N", label.serial], ["IMEI", label.imei], ["EID", label.eid]].entries()) {
      const result = await writer.writeBarcode(value, { format: "Code128", scale: 3 })
      assert.equal(result.error, "")
      const size = result.svg.match(/<svg width="(\d+)" height="(\d+)"/)
      assert(size, "Writer must return SVG dimensions")
      const content = result.svg.replace(/^.*?<svg\b[^>]*>/s, "").replace(/<\/svg>\s*$/, "")
      const y = 180 + row * 220
      parts.push(`<text x="48" y="${y}" font-size="24" font-family="monospace">${name}: ${value}</text>`)
      parts.push(`<svg x="48" y="${y + 20}" width="1024" height="150" viewBox="0 0 ${size[1]} ${size[2]}" preserveAspectRatio="none">${content}</svg>`)
    }
    parts.push("</g></svg>")
    const svg = parts.join("\n")
    const png = await sharp(Buffer.from(svg)).png().toBuffer()
    const decoded = await reader.readBarcodes(new Uint8Array(png), { tryHarder: true, maxNumberOfSymbols: 8 })
    assert.deepEqual(decoded.map((item) => item.text).sort(), [label.serial, label.imei, label.eid].sort(), `${label.model}: all three barcodes must decode`)
    await fs.writeFile(path.join(output, `${label.file}.svg`), svg)
    await fs.writeFile(path.join(output, `${label.file}.png`), png)
    labels.push(label)
  }
  await fs.writeFile(path.join(output, "manifest.json"), JSON.stringify({ test_only: true, labels }, null, 2) + "\n")
  await fs.writeFile(path.join(output, "README.txt"), [
    "仅供测试 / TEST ONLY",
    `${labels.length} 个型号，每个型号一张 PNG 和一张 SVG 标签。每张包含三个 Code128 条形码。`,
    "号码均为模拟数据，不代表真实设备，也不证明对应型号支持 eSIM/EID。",
    "IMEI 使用模拟 TAC 00000000 并含 Luhn 校验位；EID 仅模拟 32 位数字，不保证行业校验有效。",
    "容量和颜色取自 imports/swappie-iphone-catalog.json；不是在线库存查询。",
    "manifest.json 为机型与号码对照清单。生成时已逐张读取 PNG 并核对全部三个条码。",
    "重新生成：SHARP_MODULE_PATH=<sharp 模块路径> node scripts/generate-device-test-labels.cjs [输出目录]",
    "",
  ].join("\n"))
  console.log(`Generated and decoded ${labels.length} labels (${labels.length * 3} Code128 barcodes): ${output}`)
}

main().catch((error) => { console.error(error); process.exitCode = 1 })
