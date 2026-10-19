import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"
import type { ConfigModule, ExecArgs, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"
import { prepareZXingModule, writeBarcode } from "zxing-wasm/writer"

export default async function checkDeviceMultiScan({ container }: ExecArgs) {
  assert(process.env.PLAYWRIGHT_MODULE_PATH)
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
  const serial = "DEVICECHECK123"
  const imei = "012345678901234"
  const eid = "89049032000000000000000000000001"
  prepareZXingModule({ overrides: { wasmBinary: new Uint8Array(readFileSync(require.resolve("zxing-wasm/writer/zxing_writer.wasm"))) } })
  const svgs: string[] = []
  for (const [index, value] of [serial, imei, eid].entries()) {
    const result = await writeBarcode(value, { format: index === 1 ? "QRCode" : "Code128", scale: 3 })
    assert.equal(result.error, "")
    svgs.push(`data:image/svg+xml;base64,${Buffer.from(result.svg).toString("base64")}`)
  }
  const config: ConfigModule = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const users: IUserModuleService = container.resolve(Modules.USER)
  const [user] = await users.listUsers({})
  const token = generateJwtToken({ actor_id: user.id, actor_type: "user", auth_identity_id: "multi_scan_check", app_metadata: { user_id: user.id } },
    { secret: config.projectConfig.http.jwtSecret, expiresIn: "5m" })
  const browser = await chromium.launch({ channel: "chrome", headless: true })
  try {
    const context = await browser.newContext({ viewport: { width: 1100, height: 900 } })
    await context.addCookies([{ name: "lng", value: "zhCN", url: "http://127.0.0.1:9001" }])
    const fixture = await context.newPage()
    await fixture.setContent(`<body style="margin:0;background:white"><main style="display:flex;flex-direction:column;align-items:center;gap:36px;padding:48px">${svgs.map((url) => `<img src="${url}" />`).join("")}</main></body>`)
    await fixture.locator("img").last().waitFor()
    await fixture.evaluate(async () => { await Promise.all([...document.images].map((image) => image.decode())) })
    const photo = resolve(process.cwd(), "../../../logs/device-three-barcode-label.png")
    await fixture.screenshot({ path: photo, fullPage: true })
    await fixture.close()
    // Supply real barcode pixels as a camera stream; exercise the actual decoder.
    await context.addInitScript((frames: string[]) => {
      Object.defineProperty(navigator.mediaDevices, "getUserMedia", { configurable: true, value: async () => {
        const images = await Promise.all(frames.map((url) => new Promise<HTMLImageElement>((done) => {
          const image = new Image(); image.onload = () => done(image); image.src = url
        })))
        const canvas = document.createElement("canvas")
        canvas.width = 1100; canvas.height = 800
        const draw = (index: number) => {
          const ctx = canvas.getContext("2d")!
          ctx.fillStyle = "white"; ctx.fillRect(0, 0, canvas.width, canvas.height)
          ctx.drawImage(images[index], 50, 50)
        }
        draw(0)
        const stream = canvas.captureStream(8)
        const cameraWindow = window as Window & { scannerFrameLimit?: number }
        let frame = 0
        const timer = setInterval(() => {
          if (stream.getVideoTracks()[0].readyState === "ended") { clearInterval(timer); return }
          draw(++frame % (cameraWindow.scannerFrameLimit || images.length))
        }, 800)
        return stream
      } })
    }, svgs)
    const page = await context.newPage()
    const errors: string[] = []
    page.on("pageerror", (error: Error) => errors.push(error.message))
    page.on("console", (message: any) => { if (message.type() === "error") console.log("Browser error:", message.text()) })
    page.on("requestfailed", (request: any) => console.log("Request failed:", request.url(), request.failure()?.errorText))
    await page.route("**/admin/**", async (route: any) => {
      if (["fetch", "xhr"].includes(route.request().resourceType()))
        await route.continue({ headers: { ...route.request().headers(), authorization: `Bearer ${token}` } })
      else await route.continue()
    })
    for (let attempt = 0; attempt < 25; attempt++) {
      try { if ((await context.request.get("http://127.0.0.1:9001/health", { timeout: 1000 })).ok()) break } catch { /* source reload */ }
      await new Promise((done) => setTimeout(done, 1000))
    }
    await page.goto("http://127.0.0.1:9001/app/device-scan/intake")
    const serialField = page.getByRole("textbox", { name: "苹果序列号", exact: true })
    const first = page.getByRole("textbox", { name: "IMEI / EID 1", exact: true })
    const second = page.getByRole("textbox", { name: "IMEI / EID 2", exact: true })
    await serialField.waitFor()
    await page.getByLabel("上传标签图片", { exact: true }).setInputFiles(photo)
    await page.waitForFunction(() => (document.querySelector('input[aria-label="苹果序列号"]') as HTMLInputElement)?.value === "DEVICECHECK123")
    assert.deepEqual([await first.inputValue(), await second.inputValue()].sort(), [imei, eid].sort())
    // Repeated images deduplicate; pre-existing serials are never silently replaced.
    await serialField.fill("KEEP-EXISTING")
    await page.getByLabel("上传标签图片", { exact: true }).setInputFiles(photo)
    await page.getByText("部分号码有冲突、数量超出或无法分类。已保留原内容，请核对序列号和两个 IMEI / EID。", { exact: true }).waitFor()
    assert.equal(await serialField.inputValue(), "KEEP-EXISTING")
    await serialField.fill(""); await first.fill(""); await second.fill("")
    await page.getByRole("button", { name: "打开手机相机", exact: true }).click()
    await page.waitForFunction(() => (document.querySelector('input[aria-label="IMEI / EID 2"]') as HTMLInputElement)?.value.length > 0)
    assert.equal(await serialField.inputValue(), serial)
    assert.deepEqual([await first.inputValue(), await second.inputValue()].sort(), [imei, eid].sort())
    await page.getByRole("button", { name: "打开手机相机", exact: true }).waitFor()
    assert(await page.evaluate(() => {
      const stream = document.querySelector("video")?.srcObject as MediaStream
      return stream.getTracks().every((track) => track.readyState === "ended")
    }), "Successful multi-scan releases the camera")
    await serialField.fill(""); await first.fill(""); await second.fill("")
    await page.evaluate(() => { (window as Window & { scannerFrameLimit?: number }).scannerFrameLimit = 2 })
    await page.getByRole("button", { name: "打开手机相机", exact: true }).click()
    await page.getByRole("status").filter({ hasText: "已识别 2 个码" }).waitFor()
    await page.getByRole("button", { name: "填入已识别号码", exact: true }).click()
    assert.equal(await serialField.inputValue(), serial)
    assert.equal(await first.inputValue(), imei)
    assert.equal(await second.inputValue(), "")
    await page.evaluate(() => { (window as Window & { scannerFrameLimit?: number }).scannerFrameLimit = 3 })
    await serialField.fill(""); await first.fill(""); await second.fill("")
    await page.getByRole("button", { name: "扫描苹果序列号", exact: true }).click()
    await page.getByRole("dialog").getByLabel("上传标签图片", { exact: true }).setInputFiles(photo)
    await page.getByRole("dialog").waitFor({ state: "hidden" })
    assert.equal(await serialField.inputValue(), serial)
    assert.deepEqual([await first.inputValue(), await second.inputValue()].sort(), [imei, eid].sort())
    await page.setViewportSize({ width: 390, height: 844 })
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), "Multi-scan does not overflow mobile")
    await page.screenshot({ path: resolve(process.cwd(), "../../../logs/device-multi-scan-mobile.png"), fullPage: true })
    assert.deepEqual(errors, [])
    console.log("PASS: three mixed barcodes from one photo, continuous camera collection, duplicate suppression, existing-field protection, camera release and mobile layout. No devices created.")
  } finally { await browser.close() }
}
