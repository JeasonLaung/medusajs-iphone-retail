const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const { prepareZXingModule, writeBarcode } = require("zxing-wasm/writer")
const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)

// UI-only check: mock the temporary session and never receive actual devices.
async function main() {
  prepareZXingModule({ overrides: { wasmBinary: new Uint8Array(fs.readFileSync(require.resolve("zxing-wasm/writer/zxing_writer.wasm"))) } })
  const identities = ["Q7M2K9V4XP", "490154203237518", "112345678901234", "89049032000000000000000000000001"]
  const frames = []
  for (const value of identities) {
    const result = await writeBarcode(value, { format: "Code128", scale: 3 })
    assert.equal(result.error, "")
    frames.push(`data:image/svg+xml;base64,${Buffer.from(result.svg).toString("base64")}`)
  }
  const browser = await chromium.launch({ channel: "chrome", headless: true })
  try {
    const context = await browser.newContext({ viewport: { width: 390, height: 844 } })
    await context.addInitScript((frames) => {
      window.cameraSample = 0
      Object.defineProperty(navigator.mediaDevices, "getUserMedia", { configurable: true, value: async () => {
        const images = await Promise.all(frames.map(url => new Promise((resolve, reject) => {
          const image = new Image(); image.onload = () => resolve(image); image.onerror = reject; image.src = url
        })))
        const canvas = document.createElement("canvas")
        canvas.width = 1920; canvas.height = 1080
        const context = canvas.getContext("2d")
        const draw = () => {
          context.fillStyle = "white"; context.fillRect(0, 0, 1920, 1080)
          // Deliberately off-center, including a portrait barcode. The old
          // JS decoder's center scanline would miss these frames.
          const image = images[window.cameraSample]
          if (image) {
            if (window.cameraSample === 1) {
              context.save(); context.translate(1550, 100); context.rotate(Math.PI / 2)
              context.drawImage(image, 0, 0); context.restore()
            } else context.drawImage(image, 110, 90)
          }
        }
        draw()
        const stream = canvas.captureStream(8)
        window.lastScanStream = stream
        const timer = setInterval(() => {
          if (stream.getTracks()[0].readyState === "ended") { clearInterval(timer); return }
          draw()
        }, 120)
        return stream
      } })
    }, frames)
    const page = await context.newPage()
    const errors = []
    page.on("pageerror", error => errors.push(error.message))
    await page.route("**/mobile-intake", route => {
      assert.equal(route.request().method(), "GET")
      return route.fulfill({ json: { source: { id: "scanner-ui-only", name: "Scanner UI test" }, category: "sale", expires_at: Date.now() + 600000 } })
    })
    for (let attempt = 0; attempt < 30; attempt++) {
      try { if ((await context.request.get("http://127.0.0.1:9001/health", { timeout: 1000 })).ok()) break } catch { /* Development source reload. */ }
      await new Promise(resolve => setTimeout(resolve, 1000))
    }
    for (const lang of ["zh", "en"]) {
      await page.goto(`http://127.0.0.1:9001/static/mobile-intake/index.html?lang=${lang}#scanner-ui-only`)
      const names = lang === "zh" ? ["扫描序列号", "扫描 IMEI 1", "扫描 IMEI 2", "扫描 EID"] : ["Scan serial number", "Scan IMEI 1", "Scan IMEI 2", "Scan EID"]
      const fields = ["serial", "imei", "imei2", "eid"].map(name => page.locator(`input[name='${name}']`))
      await fields[0].waitFor()
      assert.equal(await page.evaluate(() => Boolean(window.ZXingWASM)), false, "Decoder must be lazy loaded")
      for (let index = 0; index < 4; index++) {
        for (const field of fields) await field.fill("")
        await page.evaluate(sample => { window.cameraSample = sample }, index)
        await page.getByRole("button", { name: names[index], exact: true }).click()
        await page.waitForFunction(({ index, identities }) => document.querySelectorAll("input[name='serial'],input[name='imei'],input[name='imei2'],input[name='eid']")[index]?.value === identities[index], { index, identities })
        for (let other=0; other<4; other++) assert.equal(await fields[other].inputValue(), other === index ? identities[index] : "", "Single scan must fill only its target")
        assert(await page.evaluate(() => window.lastScanStream.getTracks().every(track => track.readyState === "ended")), "Camera must stop after a single scan")
        await page.locator(".van-overlay").waitFor({ state: "hidden" })
      }
      // Wrong identifier type must keep scanning without filling the serial.
      await fields[0].fill("")
      await page.evaluate(() => { window.cameraSample = 1 })
      await page.getByRole("button", { name: names[0], exact: true }).click()
      await page.waitForFunction(() => document.querySelector("video")?.readyState >= 2)
      await page.waitForTimeout(700)
      assert.equal(await fields[0].inputValue(), "")
      await page.evaluate(() => { window.cameraSample = 0 })
      await page.waitForFunction(value => document.querySelector("input[name='serial']")?.value === value, identities[0])
      await page.locator(".van-overlay").waitFor({ state: "hidden" })
      // The continuous dock action still accepts all three single-code frames.
      for (const field of fields) await field.fill("")
      await page.evaluate(() => { window.cameraSample = 0 })
      await page.getByRole("button", { name: lang === "zh" ? "打开摄像头扫码" : "Open camera scanner", exact: true }).click()
      for (let index=0;index<4;index++) {
        await page.evaluate(sample => { window.cameraSample = sample }, index)
        await page.waitForFunction(value => [...document.querySelectorAll("input")].some(input => input.value === value), identities[index])
      }
      await page.screenshot({ path: path.resolve(__dirname, `../../../../logs/mobile-scanner-fixed-${lang}.png`), fullPage: true })
      await page.getByRole("button", { name: lang === "zh" ? "完成扫描" : "Done scanning", exact: true }).click()
      await page.locator(".van-overlay").waitFor({ state: "hidden" })
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
    }
    assert.deepEqual(errors, [])
    console.log("PASS: standalone mobile page, off-center/portrait serial and IMEI Code128, EID Code128, all four field targets, wrong-type rejection, continuous collection, camera cleanup, Chinese and English. No inventory changes.")
  } finally { await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
