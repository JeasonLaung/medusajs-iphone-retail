import assert from "node:assert/strict"
import type { ConfigModule, ExecArgs, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"
import en from "../admin/i18n/json/en.json"
import zh from "../admin/i18n/json/zh-CN.json"

export default async function checkDeviceEditUi({ container }: ExecArgs) {
  const config: ConfigModule = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const users: IUserModuleService = container.resolve(Modules.USER)
  const [user] = await users.listUsers({})
  const token = generateJwtToken({ actor_id: user.id, actor_type: "user", auth_identity_id: "device_edit_ui_check", app_metadata: { user_id: user.id } }, { secret: config.projectConfig.http.jwtSecret, expiresIn: "5m" })
  assert(process.env.PLAYWRIGHT_MODULE_PATH)
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
  const browser = await chromium.launch({ channel: "chrome", headless: true })
  try {
    for (const [locale, copy] of [["zhCN", zh], ["en", en]] as const) {
      const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } })
      await context.addCookies([{ name: "lng", value: locale, url: "http://localhost:9001" }])
      const page = await context.newPage()
      const errors: string[] = []
      page.on("pageerror", (error: Error) => errors.push(error.message))
      const device = { id: "dev_edit_preview", serial_number: "EDITPREVIEW123", imei: "123456789012345", imei2: null, eid: null, variant_id: "variant_preview", source_id: "source_preview", location_id: "loc_preview", category: "sale", state: "available", grade: "A", battery_health: 90, cost_cents: 12345, currency_code: "eur", bin: "A1", notes: "preview", photos: [], created_at: "2026-10-20T00:00:00Z", updated_at: "2026-10-20T00:00:00Z", can_delete: true, can_retire: true }
      const variant = { id: device.variant_id, sku: "PREVIEW-SKU", title: "128 GB", product: { title: "iPhone preview" }, options: [], inventory_items: [] }
      let submitted: Record<string, unknown> | undefined
      await page.route("**/admin/**", async (route: any) => {
        const request = route.request()
        const url = new URL(request.url())
        if (url.pathname === "/admin/devices") {
          if (request.method() === "POST") {
            submitted = request.postDataJSON()
            return route.fulfill({ json: { result: {} } })
          }
          return route.fulfill({ json: { devices: [device], count: 1, offset: 0, limit: 50, access: { role: "admin", source_id: null, order_ids: [] }, variants: [variant], sources: [{ id: device.source_id, location_id: device.location_id, name: "Preview warehouse", type: "self" }], assignments: [], assigned_devices: [], events: [], supplies: [], managed_variants: [] } })
        }
        if (url.pathname === "/admin/devices/variants") return route.fulfill({ json: { variants: [variant], count: 1, offset: 0, limit: 20 } })
        if (["fetch", "xhr"].includes(request.resourceType())) return route.continue({ headers: { ...request.headers(), authorization: `Bearer ${token}` } })
        return route.continue()
      })
      await page.goto("http://localhost:9001/app/device-scan/lookup")
      const edit = page.getByRole("button", { name: copy.deviceScan.editDevice, exact: true })
      await edit.waitFor({ timeout: 45000 })
      await edit.click()
      const drawer = page.getByRole("dialog").last()
      const serial = drawer.getByRole("textbox", { name: copy.orderDevices.serial, exact: true })
      await serial.waitFor()
      assert.equal(await serial.inputValue(), device.serial_number)
      assert.equal(await serial.getAttribute("readonly"), "")
      await drawer.getByRole("textbox", { name: copy.deviceScan.intakeNotes, exact: true }).fill("edited preview")
      await drawer.getByRole("button", { name: copy.deviceScan.editSave, exact: true }).click()
      await page.getByText(copy.deviceScan.editSaved, { exact: true }).waitFor()
      assert(submitted)
      assert.equal(submitted.notes, "edited preview")
      assert.equal(submitted.cost_cents, 12345)
      assert.equal(submitted.action, "update")
      assert(!("serial_number" in submitted))
      assert.equal(errors.length, 0, errors.join("\n"))
      console.log(`Device edit UI passed: ${locale}; serial read-only, editable notes, serial omitted on submit`)
      await context.close()
    }
  } finally { await browser.close() }
}
