import assert from "node:assert/strict"
import { resolve } from "node:path"
import type { ConfigModule, ExecArgs, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"
import en from "../admin/i18n/json/en.json"
import zh from "../admin/i18n/json/zh-CN.json"

// Read the existing order; intercept every write so this check never allocates real stock.
export default async function checkAllocationDialogUi({ container }: ExecArgs) {
  assert(process.env.PLAYWRIGHT_MODULE_PATH)
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
  const config: ConfigModule = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const [user] = await container.resolve<IUserModuleService>(Modules.USER).listUsers({})
  const token = generateJwtToken({ actor_id: user.id, actor_type: "user", auth_identity_id: "allocation_ui_check", app_metadata: { user_id: user.id } }, { secret: config.projectConfig.http.jwtSecret, expiresIn: "10m" })
  const orderId = "order_01M59D0VTFETD93F7S84Z8P13H"
  const listing = await fetch(`http://localhost:9001/admin/devices?order_id=${orderId}`, { headers: { authorization: `Bearer ${token}` } }).then((response) => response.json())
  const fixture = { id: "dev_dialog_preview", serial_number: "DIALOG-PREVIEW", imei: "123456789012345", imei2: null, eid: null, variant_id: listing.order.items[0].variant_id, source_id: listing.sources[0].id, location_id: listing.order.fulfillments[0].location_id, category: "sale", state: "available", grade: "S", battery_health: 85, bin: "" }
  const browser = await chromium.launch({ channel: "chrome", headless: true })
  try {
    for (const [locale, copy] of [["zhCN", zh], ["en", en]] as const) {
      const context = await browser.newContext({ viewport: { width: 1440, height: 1100 } })
      await context.addCookies([{ name: "lng", value: locale, url: "http://localhost:9001" }])
      const page = await context.newPage()
      page.setDefaultTimeout(30000)
      const errors: string[] = []
      page.on("pageerror", (error: Error) => errors.push(error.message))
      const writes: any[] = []
      await page.route("**/admin/**", async (route: any) => {
        const request = route.request()
        const url = new URL(request.url())
        if (request.method() !== "GET") {
          if (new URL(request.url()).pathname === "/admin/devices") writes.push(request.postDataJSON())
          return route.fulfill({ json: { result: {} } })
        }
        if (url.pathname === "/admin/devices" && (url.searchParams.has("available_variant_id") || url.searchParams.get("device_id") === fixture.id)) {
          return route.fulfill({ json: { ...listing, devices: [fixture], count: 1 } })
        }
        return route.continue({ headers: { ...request.headers(), authorization: `Bearer ${token}` } })
      })
      await page.goto(`http://localhost:9001/app/orders/${orderId}`)
      const action = page.getByRole("button", { name: copy.orderDevices.manageAllocation, exact: true }).first()
      await action.waitFor({ timeout: 60000 })
      // The action is inside the native fulfillment card, not a separate widget.
      assert(await action.evaluate((button: HTMLElement) => Boolean(button.closest("[data-order-allocation-host]")?.parentElement?.querySelector("h2"))))
      await action.click()
      let dialog = page.getByRole("dialog").last()
      try { await dialog.getByText(copy.orderDevices.stockOnlyHelp, { exact: true }).waitFor() }
      catch (error) {
        await page.screenshot({ path: resolve(process.cwd(), "../../../logs/allocation-dialog-failure.png"), fullPage: true })
        console.log("Dialog debug:", await page.getByRole("dialog").allTextContents(), "Errors:", errors)
        throw error
      }
      assert.equal(await dialog.getByText("orderDevices.stockOnlyHelp", { exact: true }).count(), 0)
      const choose = dialog.getByRole("button", { name: copy.orderDevices.selectDevice, exact: true }).first()
      await choose.waitFor()
      assert(await choose.isEnabled(), "Unshipped native fulfillment must accept a device")
      await choose.click()
      const save = dialog.getByRole("button", { name: copy.orderDevices.confirmAllocation, exact: true })
      await save.waitFor()
      assert(await save.isEnabled())
      assert.equal(writes.length, 0, "Selecting a device must not save")
      await dialog.getByRole("button", { name: locale === "zhCN" ? "取消" : "Cancel", exact: true }).click()
      assert.equal(writes.length, 0, "Cancel must not save")
      await action.click()
      dialog = page.getByRole("dialog").last()
      await dialog.getByRole("button", { name: copy.orderDevices.selectDevice, exact: true }).first().click()
      await dialog.screenshot({ path: resolve(process.cwd(), `../../../logs/allocation-dialog-${locale}.png`) })
      await dialog.getByRole("button", { name: copy.orderDevices.confirmAllocation, exact: true }).click()
      await page.getByText(copy.orderDevices.allocationSaved, { exact: true }).waitFor()
      assert.equal(writes.length, 1)
      assert.equal(writes[0].action, "save_allocation")
      assert.equal(writes[0].allocations.length, 1)
      assert(writes[0].allocations[0].fulfillment_id)
      assert.equal(errors.length, 0, errors.join("\n"))
      console.log(`${locale}: fulfillment portal, translated dialog, enabled selection, cancel without writes and explicit save passed`)
      await context.close()
    }
  } finally { await browser.close() }
}
