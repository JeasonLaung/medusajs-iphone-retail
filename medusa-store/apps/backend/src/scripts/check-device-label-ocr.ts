import assert from "node:assert/strict"
import { resolve } from "node:path"
import { readFileSync } from "node:fs"
import type { ConfigModule, ExecArgs, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"

// Read-only browser checks: no device or inventory requests are submitted.
export default async function checkDeviceLabelOcr({ container }: ExecArgs) {
  assert(process.env.PLAYWRIGHT_MODULE_PATH)
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
  const config: ConfigModule = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const [user] = await container.resolve<IUserModuleService>(Modules.USER).listUsers({})
  assert(user)
  const token = generateJwtToken({ actor_id: user.id, actor_type: "user", auth_identity_id: "label_ocr_check", app_metadata: { user_id: user.id } },
    { secret: config.projectConfig.http.jwtSecret, expiresIn: "10m" })
  const browser = await chromium.launch({ channel: "chrome", headless: true })
  const photo = resolve(process.cwd(), "../../../logs/iphone-15-three-code-demo.png")
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1050 } })
    await context.addCookies([{ name: "lng", value: "zhCN", url: "http://127.0.0.1:9001" }])
    const page = await context.newPage()
    const errors: string[] = []
    const external: string[] = []
    page.on("pageerror", (error: Error) => errors.push(error.message))
    page.on("console", (message: any) => { if (message.type() === "error") console.log("Browser error:", message.text().slice(0,300)) })
    page.on("requestfailed", (request: any) => console.log("Failed request:", request.url().split("?")[0], request.failure()?.errorText))
    page.on("request", (request: any) => {
      if (/^https?:/.test(request.url()) && !request.url().startsWith("http://127.0.0.1:9001/")) external.push(request.url())
    })
    await page.route("**/admin/**", async (route: any) => {
      if (["xhr", "fetch"].includes(route.request().resourceType())) {
        assert.equal(route.request().method(), "GET", "This check must not mutate admin data")
        await route.continue({ headers: { ...route.request().headers(), authorization: `Bearer ${token}` } })
      } else await route.continue()
    })
    for (let attempt = 0; attempt < 30; attempt++) {
      try { if ((await context.request.get("http://127.0.0.1:9001/health", { timeout: 1000 })).ok()) break } catch { /* Source edits may restart development. */ }
      await new Promise((done) => setTimeout(done, 1000))
    }
    for (let attempt = 0; attempt < 3; attempt++) {
      await page.goto("http://127.0.0.1:9001/app/device-scan/intake")
      try { await page.getByTestId("device-scan-page").waitFor({ timeout: 15000 }); break } catch { if (attempt === 2) throw new Error(`Admin failed to load: ${errors.join("; ")}`) }
    }
    const scanner = page.getByTestId("device-scanner").first()
    const serial = page.getByRole("textbox", { name: "苹果序列号", exact: true })
    const first = page.getByRole("textbox", { name: "IMEI / EID 1", exact: true })
    const second = page.getByRole("textbox", { name: "IMEI / EID 2", exact: true })
    const search = page.getByRole("combobox", { name: "搜索商品 SKU", exact: true })
    await scanner.getByLabel("上传标签图片", { exact: true }).setInputFiles(photo)
    try { await search.waitFor({ timeout: 60000 }) } catch (error) {
      console.log("Scanner state:", await scanner.innerText(), "Page errors:", errors)
      await page.screenshot({ path: resolve(process.cwd(), "../../../logs/device-label-ocr-failure.png") })
      throw error
    }
    assert.equal(await search.inputValue(), "iphone 15 128 yellow")
    assert.equal(await serial.inputValue(), "Q7M2K9V4XP")
    assert.deepEqual([await first.inputValue(), await second.inputValue()].sort(), ["490154203237518", "89049032000000000000000000000001"].sort())
    const options = page.getByRole("listbox", { name: "商品 SKU", exact: true }).getByRole("option")
    await options.first().waitFor()
    assert((await options.allTextContents()).every((text: string) => /iPhone 15/.test(text) && /128/.test(text) && /yellow/i.test(text)))
    assert.equal(await page.getByRole("button", { name: "商品 SKU", exact: true }).innerText(), "请选择", "OCR must not choose a SKU automatically")
    await page.screenshot({ path: resolve(process.cwd(), "../../../logs/device-label-ocr-desktop.png") })
    await page.keyboard.press("Escape")
    // Same image again must replace the search prefill, rather than relying on a changed string.
    await page.getByRole("button", { name: "商品 SKU", exact: true }).click()
    await search.fill("manual-filter")
    await page.keyboard.press("Escape")
    await scanner.getByLabel("上传标签图片", { exact: true }).setInputFiles(photo)
    await search.waitFor({ timeout: 60000 })
    assert.equal(await search.inputValue(), "iphone 15 128 yellow")
    await page.keyboard.press("Escape")
    await serial.fill(""); await first.fill(""); await second.fill("")
    // A sideways label exercises OCR rotation and preserves barcode identities.
    const fixture = await context.newPage()
    await fixture.setContent(`<body style="margin:0;width:1200px;height:2000px;overflow:hidden"><img src="data:image/png;base64,${readFileSync(photo).toString("base64")}" style="width:2000px;height:1200px;transform-origin:0 0;transform:translateX(1200px) rotate(90deg)"></body>`)
    await fixture.evaluate(async () => { await document.images[0].decode() })
    const sideways = await fixture.screenshot({ fullPage: true })
    await fixture.close()
    await scanner.getByLabel("上传标签图片", { exact: true }).setInputFiles({ name: "sideways.png", mimeType: "image/png", buffer: sideways })
    await search.waitFor({ timeout: 60000 })
    assert.equal(await search.inputValue(), "iphone 15 128 yellow")
    assert.equal(await serial.inputValue(), "Q7M2K9V4XP")
    await page.setViewportSize({ width: 390, height: 844 })
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
    await page.screenshot({ path: resolve(process.cwd(), "../../../logs/device-label-ocr-mobile.png") })
    await page.keyboard.press("Escape")
    // Loading failure must preserve barcode filling and explain the SKU fallback.
    await serial.fill(""); await first.fill(""); await second.fill("")
    await page.route("**/*eng.traineddata*", (route: any) => route.abort())
    await scanner.getByLabel("上传标签图片", { exact: true }).setInputFiles(photo)
    await page.getByText("商品文字识别失败，请手动搜索 SKU。已识别的号码仍可使用。", { exact: true }).waitFor()
    assert.equal(await serial.inputValue(), "Q7M2K9V4XP")
    await page.unroute("**/*eng.traineddata*")
    await context.addCookies([{ name: "lng", value: "en", url: "http://127.0.0.1:9001" }])
    await page.reload()
    await page.getByText("Read the iPhone model, capacity and English color from the label to prefill SKU search. Verify the details and choose a SKU.", { exact: true }).waitFor()
    assert.deepEqual(external, [], "OCR assets must remain local")
    assert.deepEqual(errors, [])
    console.log("Browser checks passed: real local OCR, all three identifiers, filtered SKU prefill without selection, repeated image, sideways label, mobile layout, OCR failure fallback and English copy")
  } finally { await browser.close() }
}
