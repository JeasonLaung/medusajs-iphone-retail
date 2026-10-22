/* Fetch Swappie buyback price matrices slowly through a real browser context.
 * Usage: node scripts/fetch-swappie-buyback-prices.cjs [delayMs]
 */
const fs = require("node:fs/promises")
const path = require("node:path")
const crypto = require("node:crypto")
const { chromium } = require("playwright")

const root = path.resolve(__dirname, "..")
const outputDir = path.join(root, "imports", "swappie-buyback-prices")
const catalogPath = path.join(root, "imports", "swappie-iphone-catalog.json")
const delayMs = Math.max(3000, Number(process.argv[2] || 5000))
const storages = ["256GB", "512GB", "1024GB", "2048GB"]
const endpoint = "https://swappie.com/api/sell/api/v3/prices/"

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const stamp = () => new Date().toISOString()
const digest = (value) => crypto.createHash("sha256").update(value).digest("hex")

async function main() {
  const catalog = JSON.parse(await fs.readFile(catalogPath, "utf8"))
  const models = [...new Set(catalog.products.map((item) => item.model).filter(Boolean))]
  await fs.mkdir(outputDir, { recursive: true })
  const browser = await chromium.launch({ channel: "chrome", headless: true })
  const page = await browser.newPage({
    userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131 Safari/537.36",
  })
  const manifest = {
    source: endpoint,
    country: "BE",
    storages,
    variants: [],
    requested_at: stamp(),
    delay_ms: delayMs,
    models,
    files: [],
  }
  try {
    for (let index = 0; index < models.length; index += 1) {
      const model = models[index]
      const url = `${endpoint}?model_name=${encodeURIComponent(model)}&country=BE&storages=${encodeURIComponent(JSON.stringify(storages))}&variants=${encodeURIComponent(JSON.stringify([]))}`
      const startedAt = stamp()
      let status = null
      let body = ""
      let error = null
      try {
        const response = await page.goto(url, { waitUntil: "domcontentloaded", timeout: 60000 })
        status = response?.status() ?? null
        body = await page.locator("body").innerText()
        const parsed = JSON.parse(body)
        const file = `${String(index + 1).padStart(2, "0")}-${model.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "")}.json`
        await fs.writeFile(path.join(outputDir, file), JSON.stringify({
          model,
          country: "BE",
          storages,
          variants: [],
          endpoint: url,
          fetched_at: startedAt,
          http_status: status,
          response_sha256: digest(body),
          ...parsed,
        }, null, 2) + "\n")
        manifest.files.push({ model, file, fetched_at: startedAt, http_status: status, total: parsed.total ?? parsed.results?.length ?? null, response_sha256: digest(body) })
        console.log(`[${index + 1}/${models.length}] ${model}: ${status}, ${parsed.total ?? parsed.results?.length ?? 0} rows`)
      } catch (cause) {
        error = String(cause?.stack || cause)
        manifest.files.push({ model, fetched_at: startedAt, http_status: status, error })
        console.error(`[${index + 1}/${models.length}] ${model}: ERROR ${error}`)
      }
      if (index < models.length - 1) await sleep(delayMs)
    }
  } finally {
    await browser.close()
  }
  manifest.completed_at = stamp()
  await fs.writeFile(path.join(outputDir, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n")
  const ok = manifest.files.filter((file) => file.http_status === 200 && !file.error).length
  console.log(`Saved ${ok}/${models.length} model responses to ${outputDir}`)
}

main().catch((error) => { console.error(error); process.exitCode = 1 })
