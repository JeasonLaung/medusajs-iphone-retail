import assert from "node:assert/strict"
import { mkdirSync, readFileSync } from "node:fs"
import { resolve } from "node:path"
import type { ConfigModule, ExecArgs, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"
import en from "../admin/i18n/json/en.json"
import zh from "../admin/i18n/json/zh-CN.json"

// Browser checks intercept intake writes; no devices or inventory are created.
export default async function checkDeviceIntakeUi({ container }: ExecArgs) {
  assert(process.env.PLAYWRIGHT_MODULE_PATH)
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
  const config: ConfigModule = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const [user] = await container.resolve<IUserModuleService>(Modules.USER).listUsers({})
  assert(user)
  const token = generateJwtToken({ actor_id: user.id, actor_type: "user", auth_identity_id: "intake_ui_check", app_metadata: { user_id: user.id } }, { secret: config.projectConfig.http.jwtSecret, expiresIn: "10m" })
  const browser = await chromium.launch({ channel: "chrome", headless: true })
  const screenshots = resolve(process.cwd(), "../../../artifacts/intake-split")
  mkdirSync(screenshots, { recursive: true })
  const image = readFileSync(resolve(process.cwd(), "../../../logs/device-three-barcode-label.png"))
  try {
    for (const [locale, copy] of [["zhCN", zh], ["en", en]] as const) {
      const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } })
      await context.addCookies([{ name: "lng", value: locale, url: "http://localhost:9001" }])
      const page = await context.newPage()
      page.setDefaultTimeout(45000)
      const errors: string[] = []
      page.on("pageerror", (error: Error) => errors.push(error.message))
      const variant = { id: "variant_intake_preview", sku: "INTAKE-PREVIEW", title: "128 GB / Black / A / 85%", product: { title: "iPhone 14" }, options: [], inventory_items: [] }
      const submissions: Record<string, any>[] = []
      const sessions: Record<string, string>[] = []
      await page.route("**/admin/**", async (route: any) => {
        const request = route.request()
        const path = new URL(request.url()).pathname
        if (path === "/admin/devices/mobile-intake") {
          sessions.push(request.postDataJSON())
          return route.fulfill({ json: { token: "preview-intake-token", expires_at: Date.now() + 7200000 } })
        }
        if (path === "/admin/devices") {
          if (request.method() === "POST") {
            submissions.push(request.postDataJSON())
            return route.fulfill({ json: { result: {} } })
          }
          return route.fulfill({ json: { devices: [], count: 0, offset: 0, limit: 50, access: { role: "admin", order_ids: [] }, sources: [{ id: "source_intake_preview", location_id: "loc_intake_preview", name: "Preview warehouse", type: "self" }], variants: [variant], assignments: [], assigned_devices: [], events: [], supplies: [], managed_variants: [] } })
        }
        if (path === "/admin/devices/variants") return route.fulfill({ json: { variants: [variant], count: 1, offset: 0, limit: 20 } })
        if (["fetch", "xhr"].includes(request.resourceType())) return route.continue({ headers: { ...request.headers(), authorization: `Bearer ${token}` } })
        return route.continue()
      })
      await page.route("**/static/mobile-intake/index.html?*", (route: any) => route.fulfill({ contentType: "text/html", body: "<title>Mobile intake preview</title>" }))
      // Exercise barcode filling when OCR is unavailable, without downloading language data.
      await page.route("**/*eng.traineddata*", (route: any) => route.abort())
      await page.goto("http://localhost:9001/app/device-scan/lookup")
      await page.getByRole("button", { name: copy.deviceScan.lookupImageIntake, exact: true }).click()
      let dialog = page.getByRole("dialog").first()
      await dialog.getByText(copy.deviceScan.imageIntakeTitle, { exact: true }).waitFor()
      assert.equal(await dialog.getByRole("button", { name: copy.deviceScan.startCamera, exact: true }).count(), 0)
      assert.equal(await dialog.getByText(copy.mobileIntake.title, { exact: true }).count(), 0)
      await dialog.getByLabel(copy.deviceScan.uploadLabel, { exact: true }).setInputFiles({ name: "label.png", mimeType: "image/png", buffer: image })
      const serial = dialog.getByRole("textbox", { name: copy.deviceScan.serial, exact: true })
      await page.waitForFunction((label: string) => !!(document.querySelector(`input[aria-label="${label}"]`) as HTMLInputElement)?.value, copy.deviceScan.serial)
      const serialValue = await serial.inputValue()
      assert(serialValue)
      assert((await dialog.getByRole("textbox", { name: copy.orderDevices.imei, exact: true }).inputValue()).length === 15)
      assert((await dialog.getByRole("textbox", { name: copy.orderDevices.eid, exact: true }).inputValue()).length === 32)
      await page.waitForFunction((label: string) => !document.querySelector(`input[aria-label="${label}"]`)?.matches(":disabled"), copy.deviceScan.serial)
      await page.getByText(copy.deviceScan.labelTextError, { exact: true }).waitFor({ state: "hidden" })
      await page.screenshot({ path: resolve(screenshots, `image-${locale}.png`) })
      await dialog.getByRole("button", { name: copy.deviceScan.sku, exact: true }).click()
      await page.getByRole("option").filter({ hasText: "INTAKE-PREVIEW" }).click()
      await page.getByRole("listbox").waitFor({ state: "hidden" })
      await page.getByText(copy.deviceScan.labelTextError, { exact: true }).waitFor({ state: "hidden" })
      await page.setViewportSize({ width: 390, height: 844 })
      await page.waitForTimeout(300)
      await page.screenshot({ path: resolve(screenshots, `image-mobile-${locale}.png`) })
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
      await dialog.getByRole("button", { name: copy.deviceScan.registerIntake, exact: true }).click()
      await page.getByText(copy.deviceScan.saleIntakeSuccess, { exact: true }).waitFor()
      assert.equal(submissions.length, 1)
      assert.equal(submissions[0].action, "intake")
      assert.equal(submissions[0].devices[0].serial_number, serialValue)
      assert.equal(submissions[0].devices[0].variant_id, variant.id)
      await dialog.getByRole("button", { name: copy.deviceScan.lookupClose, exact: true }).click()
      await page.getByRole("button", { name: copy.deviceScan.lookupScanIntake, exact: true }).click()
      assert.equal(await page.getByRole("menuitem").count(), 2)
      await page.getByRole("menuitem", { name: copy.deviceScan.scanIntakeQr, exact: true }).click()
      dialog = page.getByRole("dialog").first()
      await dialog.getByRole("img", { name: copy.mobileIntake.qr, exact: true }).waitFor()
      assert.equal(await dialog.getByTestId("device-scanner").count(), 0)
      assert.equal(await dialog.getByRole("textbox", { name: copy.deviceScan.serial, exact: true }).count(), 0)
      assert.equal(sessions[0].source_id, "source_intake_preview")
      assert.equal(sessions[0].category, "sale")
      assert((await dialog.getByRole("textbox", { name: copy.mobileIntake.link, exact: true }).inputValue()).endsWith("#preview-intake-token"))
      await page.screenshot({ path: resolve(screenshots, `qr-mobile-${locale}.png`) })
      await dialog.getByRole("button", { name: copy.deviceScan.lookupClose, exact: true }).click()
      await page.getByRole("button", { name: copy.deviceScan.lookupScanIntake, exact: true }).click()
      await page.getByRole("menuitem", { name: copy.deviceScan.scanIntakeDirect, exact: true }).click()
      await page.waitForURL("**/static/mobile-intake/index.html?*#preview-intake-token")
      assert.equal(new URL(page.url()).searchParams.get("lang"), locale === "en" ? "en" : "zh")
      for (const category of ["returned", "recycled"]) {
        await page.goto(`http://localhost:9001/app/device-scan/${category}`)
        await page.getByRole("button", { name: copy.deviceScan.lookupImageIntake, exact: true }).click()
        await page.getByText(copy.deviceScan.imageIntakeTitle, { exact: true }).waitFor()
        await page.getByRole("dialog").first().getByRole("button", { name: copy.deviceScan.lookupClose, exact: true }).click()
        await page.getByRole("button", { name: copy.deviceScan.lookupScanIntake, exact: true }).click()
        await page.getByRole("menuitem", { name: copy.deviceScan.scanIntakeDirect, exact: true }).click()
        await page.waitForURL("**/static/mobile-intake/index.html?*#preview-intake-token")
        assert.equal(sessions[sessions.length - 1].category, category)
      }
      await page.goto("http://localhost:9001/app/device-scan/intake")
      await page.getByRole("tab", { name: copy.deviceScan.lookupImageIntake, exact: true }).click()
      await page.getByText(copy.deviceScan.imageIntakeTitle, { exact: true }).waitFor()
      assert.equal(await page.getByText(copy.mobileIntake.title, { exact: true }).count(), 0)
      await page.getByRole("tab", { name: copy.deviceScan.lookupScanIntake, exact: true }).click()
      await page.getByRole("img", { name: copy.mobileIntake.qr, exact: true }).waitFor()
      await page.getByRole("tab", { name: copy.deviceScan.lookupImportIntake, exact: true }).click()
      await page.getByText(copy.deviceScan.csvTitle, { exact: true }).waitFor()
      assert.equal(await page.getByTestId("device-scanner").count(), 0)
      assert.deepEqual(errors, [])
      console.log(`PASS ${locale}: image intake excludes mobile links; scan menu has exactly two actions; QR auto-generation and direct navigation preserve warehouse, category and language`)
      await context.close()
    }
  } finally { await browser.close() }
}
