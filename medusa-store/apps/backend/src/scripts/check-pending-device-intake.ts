import assert from "node:assert/strict"
import type { ConfigModule, ExecArgs, IInventoryService, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"
import { deviceOperationWorkflow } from "../workflows/device-operations"
import { createIntakeToken } from "../lib/mobile-intake-token"
import { submitMobileIntakeWorkflow, deleteMobileIntakeWorkflow } from "../workflows/mobile-intake"
import zh from "../admin/i18n/json/zh-CN.json"
import en from "../admin/i18n/json/en.json"

export default async function check({ container }: ExecArgs) {
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const inventory: IInventoryService = container.resolve(Modules.INVENTORY)
  const users: IUserModuleService = container.resolve(Modules.USER)
  const actor = (await users.listUsers({})).find((user) => user.metadata?.device_role !== "supplier")!
  assert(actor)
  // Keep test stock outside all sales channels so customers cannot order it.
  const locations = container.resolve(Modules.STOCK_LOCATION)
  const location = await locations.createStockLocations({ name: `Pending check ${Date.now()}` })
  const source = await service.createDeviceSources({ name: location.name, location_id: location.id, type: "self" })
  const [managed] = await service.listDeviceVariants({})
  assert(source && managed)
  const run = (command: any) => deviceOperationWorkflow(container).run({ input: { ...command, actor_id: actor.id } })
  const stock = async () => Number((await inventory.listInventoryLevels({ inventory_item_id: managed.inventory_item_id, location_id: source.location_id }))[0]?.stocked_quantity || 0)
  const baseline = await stock()
  const prefix = `PENDINGCHECK${Date.now()}`
  const serials = [1, 2, 3].map((n) => `${prefix}${n}`)
  try {
    await run({ action: "intake", devices: serials.map((serial_number, index) => ({ serial_number, imei: `35${String(Date.now() + index).slice(-13)}`, variant_id: managed.variant_id, source_id: source.id, category: "sale", photos: [], cost_cents: 0, currency_code: "eur", bin: "", notes: "Pending intake regression check" })) })
    const devices = await service.listDevices({ serial_number: serials })
    assert.equal(devices.length, 3)
    assert(devices.every((device) => device.state === "pending"))
    assert.equal(await stock(), baseline, "Pending intake never adds stock")
    await run({ action: "make_available", device_ids: devices.slice(0, 2).map((device) => device.id) })
    assert.equal(await stock(), baseline + 2, "Two confirmations add exactly two units")
    assert((await service.listDevices({ id: devices.slice(0, 2).map((device) => device.id) })).every((device) => device.state === "available"))
    await assert.rejects(() => run({ action: "make_available", device_ids: [devices[0].id] }))
    await assert.rejects(() => run({ action: "make_available", device_ids: [devices[2].id, devices[0].id] }))
    assert.equal((await service.retrieveDevice(devices[2].id)).state, "pending", "Invalid mixed batch makes no changes")
    assert.equal(await stock(), baseline + 2, "Retries cannot double-count stock")
    const secret = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE).projectConfig.http.jwtSecret
    assert(typeof secret === "string")
    const link = createIntakeToken(actor.id, source.id, secret)
    const otherLink = createIntakeToken(actor.id, source.id, secret)
    const mobileSerial = `${prefix}MOBILE`
    serials.push(mobileSerial)
    const { result: receipt } = await submitMobileIntakeWorkflow(container).run({ input: { token: link.token, device: { serial_number: mobileSerial, imei: `36${String(Date.now()).slice(-13)}`, variant_id: managed.variant_id } as any } })
    const mobileId = receipt.device_ids[0]
    assert(mobileId)
    await assert.rejects(() => deleteMobileIntakeWorkflow(container).run({ input: { token: otherLink.token, device_id: mobileId } }))
    await assert.rejects(() => deleteMobileIntakeWorkflow(container).run({ input: { token: link.token, device_id: devices[2].id } }))
    const deleteUrl = `http://127.0.0.1:9001/mobile-intake/devices/${mobileId}`
    assert.equal((await fetch(deleteUrl, { method: "DELETE" })).status, 401, "Live route rejects missing token")
    assert.equal((await fetch(deleteUrl, { method: "DELETE", headers: { "x-intake-token": otherLink.token } })).status, 400, "Live route rejects another session")
    const deleted = await fetch(deleteUrl, { method: "DELETE", headers: { "x-intake-token": link.token } })
    assert.equal(deleted.status, 200, await deleted.text())
    assert.equal((await service.listDevices({ id: mobileId })).length, 0)
    assert.equal(await stock(), baseline + 2, "Deleting pending mobile intake does not change stock")
    console.log("VERIFIED: mobile receipt deletion and session isolation")
    console.log("VERIFIED: pending intake, bulk confirmation, repeat protection and atomic validation")
  } finally {
    for (const device of await service.listDevices({ serial_number: serials })) await run({ action: "delete", device_id: device.id })
    assert.equal(await stock(), baseline, "Test stock fully restored")
    const levels = await inventory.listInventoryLevels({ inventory_item_id: managed.inventory_item_id, location_id: source.location_id })
    if (levels.length) await inventory.deleteInventoryLevels(levels.map((level) => level.id))
    await service.deleteDeviceSources(source.id)
    await locations.deleteStockLocations(location.id)
  }
  if (!process.env.PLAYWRIGHT_MODULE_PATH) return
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
  const config: ConfigModule = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const token = generateJwtToken({ actor_id: actor.id, actor_type: "user", auth_identity_id: "pending_ui_check", app_metadata: { user_id: actor.id } }, { secret: config.projectConfig.http.jwtSecret, expiresIn: "10m" })
  const browser = await chromium.launch({ channel: "chrome", headless: true })
  try {
    for (const [locale, copy] of [["zhCN", zh], ["en", en]] as const) {
      const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } })
      await context.addCookies([{ name: "lng", value: locale, url: "http://localhost:9001" }])
      const page = await context.newPage()
      const errors: string[] = []
      page.on("pageerror", (error: Error) => errors.push(error.message))
      let published = false
      let submissions = 0
      await page.route("**/admin/**", async (route: any) => {
        const request = route.request()
        if (new URL(request.url()).pathname === "/admin/devices") {
          if (request.method() === "POST") {
            assert.deepEqual(request.postDataJSON(), { action: "make_available", device_ids: ["pending_1", "pending_2"] })
            published = true
            submissions++
            return route.fulfill({ json: { result: {} } })
          }
          return route.fulfill({ json: { access: { role: "admin", order_ids: [] }, devices: [1, 2, 3].map((n) => ({ id: `pending_${n}`, serial_number: `REVIEW-${n}`, variant_id: managed.variant_id, source_id: source.id, location_id: source.location_id, category: "sale", state: n === 3 || published ? "available" : "pending", can_delete: false, can_retire: false, photos: [], cost_cents: 0, currency_code: "eur" })), count: 3, sources: [source], variants: [], assignments: [], assigned_devices: [], events: [], supplies: [], managed_variants: [] } })
        }
        if (["fetch", "xhr"].includes(request.resourceType())) return route.continue({ headers: { ...request.headers(), authorization: `Bearer ${token}` } })
        return route.continue()
      })
      await page.goto("http://localhost:9001/app/device-scan/lookup")
      await page.getByRole("checkbox", { name: copy.deviceScan.selectPendingPage }).click()
      assert.equal(await page.getByRole("checkbox").count(), 3, "Available devices cannot be selected")
      await page.screenshot({ path: `/tmp/pending-device-${locale}.png`, fullPage: true })
      await page.getByRole("button", { name: copy.deviceScan.makeAvailableSelected.replace("{{count}}", "2") }).click()
      const dialog = page.getByRole("alertdialog")
      await dialog.getByRole("button", { name: copy.deviceScan.makeAvailable, exact: true }).click()
      await page.getByText(copy.deviceScan.makeAvailableSuccess.replace("{{count}}", "2"), { exact: true }).waitFor()
      assert.equal(submissions, 1)
      assert.deepEqual(errors, [])
      await context.close()
    }
    console.log("VERIFIED: Chinese and English pending selection and bulk availability UI")
  } finally { await browser.close() }
}
