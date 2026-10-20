import assert from "node:assert/strict"
import { resolve } from "node:path"
import type { ConfigModule, ExecArgs, IInventoryService, IUserModuleService, IFileModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"
import { createProductsWorkflow, deleteProductsWorkflow, deleteStockLocationsWorkflow } from "@medusajs/medusa/core-flows"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"
import { createDeviceSourceWorkflow } from "../workflows/device-source"

export default async function check({ container }: ExecArgs) {
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const inventory: IInventoryService = container.resolve(Modules.INVENTORY)
  const files: IFileModuleService = container.resolve(Modules.FILE)
  const users: IUserModuleService = container.resolve(Modules.USER)
  const user = (await users.listUsers({})).find((u) => u.metadata?.device_role !== "supplier")!
  const config: ConfigModule = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const token = generateJwtToken({ actor_id: user.id, actor_type: "user", auth_identity_id: "category-check", app_metadata: { user_id: user.id } }, { secret: config.projectConfig.http.jwtSecret, expiresIn: "10m" })
  const base = "http://127.0.0.1:9001"
  const headers = { Authorization: `Bearer ${token}`, "Content-Type": "application/json" }
  const api = async (path: string, body?: unknown, override = headers) => {
    const res = await fetch(base + path, { headers: override, ...(body ? { method: "POST", body: JSON.stringify(body) } : {}) })
    const data = await res.json()
    return { status: res.status, data }
  }
  const post = async (body: unknown) => { const res = await api("/admin/devices", body); assert.equal(res.status, 200, JSON.stringify(res.data)); return res.data }
  let source: any, product: any, browser: any
  const photoIds: string[] = []
  const stamp = Date.now()
  const image = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j7b8AAAAASUVORK5CYII="
  try {
    product = (await createProductsWorkflow(container).run({ input: { products: [{ title: `Category check ${stamp}`, handle: `category-check-${stamp}`, options: [{ title: "Condition", values: ["A"] }, { title: "Battery", values: ["85%"] }], variants: [{ title: "A / 85%", sku: `CATEGORY-${stamp}`, manage_inventory: true, options: { Condition: "A", Battery: "85%" }, prices: [{ currency_code: "eur", amount: 100 }] }] }] } })).result[0]
    source = (await createDeviceSourceWorkflow(container).run({ input: { name: `Category check ${stamp}`, type: "self" } })).result
    const variantId = product.variants[0].id
    for (let n = 0; n < 40; n++) { try { if ((await fetch(base + "/health")).ok) break } catch { /* watcher restart */ } await new Promise((done) => setTimeout(done, 1000)) }
    const stock = async () => Number((await inventory.listInventoryLevels({ location_id: source.location_id }))[0]?.stocked_quantity || 0)
    const photoBody = { files: [{ filename: "test.png", mime_type: "image/png", content: image }] }
    assert.equal((await api("/mobile-intake/photos", photoBody, { "Content-Type": "application/json" } as typeof headers)).status, 401, "No-token upload denied")
    assert.equal((await api("/admin/devices/photos", { files: [{ ...photoBody.files[0], content: "bm90LWEtcGljdHVyZQ==" }] })).status, 400, "Image bytes validated")
    const uploaded = await api("/admin/devices/photos", photoBody)
    assert.equal(uploaded.status, 201, JSON.stringify(uploaded.data))
    const photo = uploaded.data.photos[0]; photoIds.push(photo.id)
    assert.equal((await fetch(new URL(photo.url, base))).status, 200, "Photo visible on current host")
    const ids: Record<string, string> = {}
    for (const category of ["sale", "returned", "recycled"]) {
      const serial = `CAT${stamp}${category.toUpperCase()}`
      await post({ action: "intake", devices: [{ category, source_id: source.id, variant_id: variantId, serial_number: serial, photos: [photo] }] })
      const [device] = await service.listDevices({ serial_number: serial }); ids[category] = device.id
      assert.equal(device.category, category)
      assert.equal(device.state, category === "sale" ? "available" : "inspection")
      assert.deepEqual(device.photos, [photo])
      const result = await api(`/admin/devices?category=${category}&lookup_variant_id=${variantId}&limit=1`)
      assert.equal(result.data.count, 1); assert.equal(result.data.devices[0].id, device.id)
      assert.equal(result.data.devices[0].can_delete, true)
    }
    assert.equal(await stock(), 1, "Only normal intake adds stock")
    assert.equal((await api("/admin/devices", { action: "inspect", device_id: ids.sale, result: "pass", grade: "A", battery_health: 90, notes: "Invalid inspection" })).status, 400, "Ordinary devices do not enter inspection")
    // Lock status and historical sales both reject deletion on the server.
    await service.updateDevices({ id: ids.sale, state: "reserved" })
    assert.equal((await api("/admin/devices", { action: "delete", device_id: ids.sale })).status, 400)
    await service.updateDevices({ id: ids.sale, state: "available" })
    const history = await service.createDeviceAssignments({ device_id: ids.sale, source_id: source.id, order_id: `test-order-${stamp}`, item_id: "test-item", status: "returned" })
    assert.equal((await api("/admin/devices", { action: "delete", device_id: ids.sale })).status, 400)
    assert.equal((await api(`/admin/devices?device_id=${ids.sale}`)).data.devices[0].can_delete, false)
    await service.deleteDeviceAssignments(history.id)
    assert.equal(await stock(), 1, "Rejected actions leave stock intact")
    const link = await api("/admin/devices/mobile-intake", { source_id: source.id, category: "recycled" })
    assert.equal(link.status, 200)
    const mobileHeaders = { "x-intake-token": link.data.token, "Content-Type": "application/json" } as unknown as typeof headers
    assert.equal((await api("/mobile-intake", undefined, mobileHeaders)).data.category, "recycled")
    assert.equal((await api("/mobile-intake", { variant_id: variantId, serial_number: "INJECT", category: "sale" }, mobileHeaders)).status, 400)
    const mobilePhoto = await api("/mobile-intake/photos", photoBody, mobileHeaders)
    assert.equal(mobilePhoto.status, 201, JSON.stringify(mobilePhoto.data)); photoIds.push(mobilePhoto.data.photos[0].id)
    assert.equal((await api("/mobile-intake", { variant_id: variantId, serial_number: `PHONE${stamp}`, photos: mobilePhoto.data.photos }, mobileHeaders)).status, 200)
    const [phone] = await service.listDevices({ serial_number: `PHONE${stamp}` })
    assert.equal(phone.category, "recycled"); assert.equal(phone.state, "inspection"); assert.equal((phone.photos as unknown as unknown[]).length, 1)

    if (process.env.PLAYWRIGHT_MODULE_PATH) {
      const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
      browser = await chromium.launch({ channel: "chrome", headless: true })
      for (const lang of ["zhCN", "en"]) {
        const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, extraHTTPHeaders: { Authorization: `Bearer ${token}` } })
        await context.addCookies([{ name: "lng", value: lang, url: base }])
        const page = await context.newPage(); const errors: string[] = []
        page.on("pageerror", (error: Error) => errors.push(error.message))
        for (const route of ["lookup", "returned", "recycled"]) {
          await page.goto(`${base}/app/device-scan/${route}`)
          try { await page.locator("[data-testid='device-lookup-table']").waitFor() } catch (error) {
            console.log("UI DEBUG", route, lang, page.url(), errors, (await page.locator("body").innerText()).slice(0, 3500))
            await page.screenshot({ path: resolve(process.cwd(), "../../../logs/device-category-error.png"), fullPage: true })
            throw error
          }
          await page.getByRole("button", { name: lang === "zhCN" ? "扫码入库" : "Scan intake", exact: true }).click()
          await page.getByRole("dialog").waitFor()
          await page.getByRole("dialog").locator('input[type="file"][accept*="image/jpeg"]').first().waitFor()
          if (route === "lookup") {
            const uploadedResponse = page.waitForResponse((response: any) => response.url().endsWith("/admin/devices/photos") && response.request().method() === "POST")
            await page.getByRole("dialog").locator('input[type="file"][accept*="image/jpeg"]').first().setInputFiles({ name: "desktop.png", mimeType: "image/png", buffer: Buffer.from(image, "base64") })
            const response = await uploadedResponse; assert.equal(response.status(), 201)
            const photo = (await response.json()).photos[0]; photoIds.push(photo.id)
            await page.getByRole("dialog").locator(`img[src='${photo.url}']`).waitFor()
            await page.waitForFunction(() => !(Array.from(document.querySelectorAll("[role='dialog'] button")).find((element) => ["关闭", "Close"].includes(element.textContent || "")) as HTMLButtonElement)?.disabled)
          }
          assert(!/deviceScan\.[a-zA-Z]/.test(await page.locator("body").innerText()), "No untranslated UI keys")
          await page.getByRole("button", { name: lang === "zhCN" ? "关闭" : "Close", exact: true }).click()
          await page.getByRole("dialog").waitFor({ state: "hidden" })
          if (route === "returned") {
            const row = page.getByRole("row").filter({ hasText: `CAT${stamp}RETURNED` })
            await row.getByRole("link", { name: lang === "zhCN" ? "查看设备" : "View device", exact: true }).click()
            await page.locator("[data-testid='device-details'] img").waitFor()
            assert.equal(await page.locator("[data-testid='device-details'] img").count(), 1)
            await page.getByRole("button", { name: lang === "zhCN" ? "关闭" : "Close", exact: true }).click()
            await page.getByRole("dialog").waitFor({ state: "hidden" })
          }
          await page.screenshot({ path: resolve(process.cwd(), `../../../logs/device-${route}-${lang}-categories.png`), fullPage: true })
        }
        const deleteSerial = `UI${stamp}${lang.toUpperCase()}`
        await post({ action: "intake", devices: [{ category: "recycled", source_id: source.id, variant_id: variantId, serial_number: deleteSerial }] })
        await page.reload()
        const deleteRow = page.getByRole("row").filter({ hasText: deleteSerial })
        await deleteRow.getByRole("link", { name: lang === "zhCN" ? "删除" : "Delete", exact: true }).click()
        await page.getByRole("alertdialog").getByRole("button", { name: lang === "zhCN" ? "删除" : "Delete", exact: true }).click()
        await deleteRow.waitFor({ state: "hidden" })
        assert.deepEqual(errors, []); await context.close()
      }
    }
    for (const category of ["returned", "recycled"]) {
      await post({ action: "inspect", device_id: ids[category], result: "pass", grade: "A", battery_health: 90, notes: "Inspection passed" })
      const device = await service.retrieveDevice(ids[category]); assert.equal(device.category, "sale"); assert.equal(device.state, "available"); assert.deepEqual(device.photos, [photo])
    }
    assert.equal(await stock(), 3, "Each passed device adds one stock")
    await post({ action: "retire", device_id: ids.returned }); assert.equal(await stock(), 2)
    await post({ action: "delete", device_id: ids.sale }); assert.equal(await stock(), 1)
    assert.equal((await service.listDevices({ id: ids.sale })).length, 0)
    assert.equal((await service.listDeviceEvents({ device_id: ids.sale, action: "delete" })).length, 1, "Deletion retains audit history")
    await post({ action: "delete", device_id: ids.recycled }); assert.equal(await stock(), 0)
    await post({ action: "delete", device_id: phone.id }); assert.equal(await stock(), 0, "Inspection deletion never decrements stock")
    console.log("PASS: category intake and pagination, saleable stock, inspection promotion, photos and public URLs, mobile session category binding, locked/history deletion guards, soft deletion, retirement, audit and bilingual admin menus.")
  } finally {
    await browser?.close()
    if (source) {
      const devices = await service.listDevices({ source_id: source.id }, { take: null, withDeleted: true })
      const ids = devices.map((d) => d.id)
      if (ids.length) {
        const events = await service.listDeviceEvents({ device_id: ids }, { take: null })
        if (events.length) await service.deleteDeviceEvents(events.map((e) => e.id))
        const assignments = await service.listDeviceAssignments({ device_id: ids }, { take: null })
        if (assignments.length) await service.deleteDeviceAssignments(assignments.map((a) => a.id))
        await service.softDeleteDevices(ids)
      }
    }
    if (photoIds.length) await files.deleteFiles(photoIds)
    if (product) await deleteProductsWorkflow(container).run({ input: { ids: [product.id] } })
    if (source) { await service.deleteDeviceSources(source.id); await deleteStockLocationsWorkflow(container).run({ input: { ids: [source.location_id] } }) }
  }
}
