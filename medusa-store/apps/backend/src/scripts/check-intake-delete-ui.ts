import assert from "node:assert/strict"
import type { ConfigModule, ExecArgs, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"
import zh from "../admin/i18n/json/zh-CN.json"
import en from "../admin/i18n/json/en.json"

// UI requests for intake and deletion are intercepted; this check creates no stock.
export default async function check({ container }: ExecArgs) {
  assert(process.env.PLAYWRIGHT_MODULE_PATH)
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
  const config: ConfigModule = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const [actor] = await container.resolve<IUserModuleService>(Modules.USER).listUsers({})
  const token = generateJwtToken({ actor_id: actor.id, actor_type: "user", auth_identity_id: "intake_delete_ui", app_metadata: { user_id: actor.id } }, { secret: config.projectConfig.http.jwtSecret, expiresIn: "10m" })
  const variant = { id: "variant_preview", sku: "PREVIEW-SKU", title: "128 GB / Black / A / 85%", product: { title: "iPhone 14" }, requires_imei: true, options: [], inventory_items: [] }
  const source = { id: "source_preview", location_id: "location_preview", type: "self", name: "Preview warehouse" }
  const browser = await chromium.launch({ channel: "chrome", headless: true })
  try {
    for (const [locale, copy] of [["zhCN", zh], ["en", en]] as const) {
      const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } })
      await context.addCookies([{ name: "lng", value: locale, url: "http://localhost:9001" }])
      const page = await context.newPage()
      const errors: string[] = []
      page.on("pageerror", (error: Error) => errors.push(error.message))
      let pcIntakes = 0, pcDeletes = 0
      await page.route("**/admin/**", async (route: any) => {
        const request = route.request()
        const path = new URL(request.url()).pathname
        if (path === "/admin/devices") {
          if (request.method() === "POST") {
            const body = request.postDataJSON()
            if (body.action === "intake") pcIntakes++
            else { assert.deepEqual(body, { action: "delete", device_id: "device_preview" }); pcDeletes++ }
            return route.fulfill({ json: { result: { device_ids: ["device_preview"] } } })
          }
          return route.fulfill({ json: { access: { role: "admin", order_ids: [] }, devices: [], count: 0, sources: [source], variants: [variant], assignments: [], assigned_devices: [], events: [], supplies: [], managed_variants: [] } })
        }
        if (path === "/admin/devices/variants") return route.fulfill({ json: { variants: [variant], count: 1 } })
        if (["fetch", "xhr"].includes(request.resourceType())) return route.continue({ headers: { ...request.headers(), authorization: `Bearer ${token}` } })
        return route.continue()
      })
      await page.goto("http://localhost:9001/app/device-scan/lookup")
      await page.getByRole("button", { name: copy.deviceScan.lookupImageIntake, exact: true }).click()
      const intake = page.getByRole("dialog").first()
      await intake.getByRole("button", { name: copy.deviceScan.sku, exact: true }).click()
      await page.getByRole("option").filter({ hasText: variant.sku }).click()
      await intake.getByRole("textbox", { name: copy.deviceScan.serial, exact: true }).fill("PC-RECEIPT")
      await intake.getByRole("textbox", { name: copy.orderDevices.imei, exact: true }).fill("123456789012345")
      await intake.getByRole("button", { name: copy.deviceScan.registerIntake, exact: true }).click()
      const recent = intake.getByRole("region", { name: copy.deviceScan.recentIntakes })
      await recent.getByText("PC-RECEIPT", { exact: true }).waitFor()
      await page.waitForFunction((label: string) => !(document.querySelector(`section[aria-label="${label}"] button`) as HTMLButtonElement)?.disabled, copy.deviceScan.recentIntakes)
      await recent.scrollIntoViewIfNeeded()
      await page.screenshot({ path: `/tmp/intake-delete-pc-${locale}.png`, fullPage: true })
      await recent.getByRole("button", { name: copy.deviceScan.deleteDevice, exact: true }).click()
      await page.getByRole("alertdialog").getByRole("button", { name: copy.deviceScan.deleteDevice, exact: true }).click()
      await recent.waitFor({ state: "hidden" })
      assert.equal(pcIntakes, 1)
      assert.equal(pcDeletes, 1)
      await page.unroute("**/admin/**")
      let mobileIntakes = 0, mobileDeletes = 0, failDelete = true
      await page.route("**/mobile-intake**", async (route: any) => {
        const request = route.request()
        const path = new URL(request.url()).pathname
        if (!path.startsWith("/mobile-intake")) return route.continue()
        if (path === "/mobile-intake/variants") return route.fulfill({ json: { variants: [variant], count: 1 } })
        if (request.method() === "DELETE") {
          assert.equal(path, "/mobile-intake/devices/mobile_preview")
          mobileDeletes++
          if (failDelete) return route.fulfill({ status: 400, json: { message: "MOBILE_INTAKE_DELETE_DENIED" } })
          return route.fulfill({ json: { result: { device_ids: ["mobile_preview"] } } })
        }
        if (request.method() === "POST") { mobileIntakes++; return route.fulfill({ json: { result: { device_ids: ["mobile_preview"] } } }) }
        return route.fulfill({ json: { category: "sale", source, expires_at: Date.now() + 7200000 } })
      })
      await page.setViewportSize({ width: 390, height: 844 })
      await page.goto(`http://localhost:9001/static/mobile-intake/index.html?lang=${locale === "zhCN" ? "zh" : "en"}#preview-token`)
      const receive = page.locator(".scan-dock button[type='submit']")
      await receive.waitFor()
      assert(await receive.isDisabled())
      await page.locator(".van-field").first().click()
      await page.locator(".van-popup .van-cell").filter({ hasText: variant.sku }).first().click()
      await page.locator(".van-popup").filter({ has: page.locator(".variant-search-row") }).waitFor({ state: "hidden" })
      await page.locator("input[name='serial']").fill("MOBILE-RECEIPT")
      assert(await receive.isDisabled(), "Required iPhone identifier is missing")
      await page.locator("input[name='imei']").fill("123")
      assert(await receive.isDisabled(), "Invalid identifier keeps receive disabled")
      await page.locator("input[name='imei']").fill("123456789012345")
      assert(await receive.isEnabled())
      const scanBox = await page.locator(".floating-scan").boundingBox()
      const receiveBox = await receive.boundingBox()
      assert(scanBox && receiveBox && scanBox.y + scanBox.height < receiveBox.y, "Scanner sits above receiving button")
      assert.equal((await page.locator(".floating-scan").innerText()).trim(), "", "Floating scanner is an accessible icon")
      await page.screenshot({ path: `/tmp/intake-delete-mobile-ready-${locale}.png`, fullPage: true })
      await receive.click()
      await page.waitForFunction(() => (document.querySelector("input[name='serial']") as HTMLInputElement)?.value === "")
      assert(await receive.isDisabled())
      assert.equal(await page.locator(".history").count(), 0)
      await page.locator(".stats-trigger").click()
      await page.locator(".stats-device").waitFor()
      assert.equal(await page.locator(".stats-total strong").innerText(), "1")
      const deletion = page.locator(".stats-device button")
      await page.waitForFunction(() => { const rect = document.querySelector(".stats-popup")?.getBoundingClientRect(); return rect && rect.top > 0 && rect.bottom <= innerHeight + 2 })
      await deletion.scrollIntoViewIfNeeded()
      await page.screenshot({ path: `/tmp/intake-delete-mobile-stats-${locale}.png`, fullPage: true })
      await deletion.click()
      await page.locator(".van-dialog__cancel").click()
      assert.equal(await page.locator(".stats-device").count(), 1)
      await deletion.click()
      await page.locator(".van-dialog__confirm").click()
      await page.locator(".van-toast").filter({ hasText: locale === "zhCN" ? "只能删除" : "Only devices" }).waitFor()
      assert.equal(await page.locator(".stats-device").count(), 1, "Failed deletion retains the receipt")
      failDelete = false
      await deletion.click()
      await page.locator(".van-dialog__confirm").click()
      await page.locator(".stats-device").waitFor({ state: "hidden" })
      assert.equal(await page.locator(".stats-total strong").innerText(), "0")
      assert.equal(mobileIntakes, 1)
      assert.equal(mobileDeletes, 2)
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
      assert.deepEqual(errors, [])
      await context.close()
    }
    console.log("VERIFIED: Chinese/English PC image receipts, phone submit validation, floating scanner, statistics and delete/cancel/failure flows")
  } finally { await browser.close() }
}
