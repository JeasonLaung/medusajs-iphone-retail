import assert from "node:assert/strict"
import { resolve } from "node:path"
import type { ConfigModule, ExecArgs, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"
import { createProductsWorkflow, deleteProductsWorkflow, deleteStockLocationsWorkflow } from "@medusajs/medusa/core-flows"
import { createDeviceSourceWorkflow } from "../workflows/device-source"
import { deviceOperationWorkflow } from "../workflows/device-operations"
import QRCode from "qrcode"

export default async function checkMobileIntake({ container }: ExecArgs) {
  assert(process.env.PLAYWRIGHT_MODULE_PATH, "Set PLAYWRIGHT_MODULE_PATH")
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const users: IUserModuleService = container.resolve(Modules.USER)
  const user = (await users.listUsers({})).find((entry) => entry.metadata?.device_role !== "supplier")!
  assert(user, "An existing admin is required")
  let source: { id: string; location_id: string }
  let managed: { variant_id: string }
  let productId = ""
  const config: ConfigModule = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const admin = generateJwtToken({ actor_id: user.id, actor_type: "user", auth_identity_id: "mobile_intake_check", app_metadata: { user_id: user.id } }, { secret: config.projectConfig.http.jwtSecret, expiresIn: "10m" })
  const base = process.env.MOBILE_INTAKE_BASE_URL || "http://127.0.0.1:9001"
  const serials: string[] = []
  const api = (path: string, options: RequestInit = {}) => fetch(`${base}${path}`, options)
  let browser: any
  try {
  const stamp = Date.now()
  const { result: products } = await createProductsWorkflow(container).run({ input: { products: [{
    title: `iPhone mobile intake check ${stamp}`, handle: `mobile-intake-check-${stamp}`,
    options: [{ title: "Condition", values: ["A"] }, { title: "Battery", values: ["85%"] }],
    variants: [{ title: "A / 85%", sku: `MOBILE-CHECK-${stamp}`, manage_inventory: true,
      options: { Condition: "A", Battery: "85%" }, prices: [{ currency_code: "eur", amount: 100 }] }],
  }] } })
  productId = products[0].id
  managed = { variant_id: products[0].variants![0].id }
  const { result: createdSource } = await createDeviceSourceWorkflow(container).run({ input: { name: `Mobile intake check ${stamp}`, type: "self" } })
  source = createdSource
  await deviceOperationWorkflow(container).run({ input: { action: "enroll", actor_id: user.id, variant_id: managed.variant_id, acknowledged: true } })
  for (let attempt = 0; attempt < 30; attempt++) {
    try { if ((await api("/health")).ok) break } catch { /* development watcher restarting */ }
    await new Promise((done) => setTimeout(done, 1000))
  }
  const created = await api("/admin/devices/mobile-intake", { method: "POST", headers: { Authorization: `Bearer ${admin}`, "Content-Type": "application/json" }, body: JSON.stringify({ source_id: source.id }) })
  assert.equal(created.status, 200, await created.clone().text())
  const link = await created.json()
  const headers = { "x-intake-token": link.token, "Content-Type": "application/json" }
  assert.equal((await api("/mobile-intake")).status, 401, "No token denied")
  assert.equal((await api("/mobile-intake", { headers: { "x-intake-token": link.token + "forged" } })).status, 401, "Forged token denied")
  const session = await api("/mobile-intake", { headers })
  assert.equal(session.status, 200)
  assert.equal((await session.json()).source.id, source.id)
  const invalid = await api("/mobile-intake", { method: "POST", headers, body: JSON.stringify({ source_id: "another-warehouse", variant_id: managed.variant_id, serial_number: "MOBILECHECK" }) })
  assert.equal(invalid.status, 400, "Client cannot change the warehouse")
  browser = await chromium.launch({ channel: "chrome", headless: true, args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream"] })
    const context = await browser.newContext({ viewport: { width: 390, height: 844 }, permissions: ["camera"] })
    const page = await context.newPage()
    const errors: string[] = []
    page.on("pageerror", (error: Error) => errors.push(error.message))
    for (const lang of ["zh", "en"]) {
      await page.goto(`${base}/static/mobile-intake/index.html?lang=${lang}#${link.token}`)
      await page.locator(".scan-trigger").waitFor()
      assert.equal(await page.evaluate(() => Boolean((window as any).ZXingBrowser)), false, "Barcode library is deferred until the camera opens")
      assert.equal(await page.locator("script[src*='/app/']").count(), 0, "No admin assets")
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), "Fits phone width")
      await page.locator(".van-field").first().click()
      await page.locator(".van-popup .van-cell").first().waitFor()
      // Use an enrolled SKU so the real receiving workflow can validate it.
      const query = container.resolve(ContainerRegistrationKeys.QUERY)
      const { data: [variant] } = await query.graph({ entity: "product_variant", fields: ["id", "sku"], filters: { id: managed.variant_id } })
      await page.locator(".van-search input").fill(variant.sku)
      await page.locator(".search-button").click()
      await page.locator(".van-popup .van-cell").filter({ hasText: variant.sku }).first().click()
      const serial = `MOBILECHECK${Date.now()}${lang.toUpperCase()}`
      serials.push(serial)
      await page.locator("input[name='serial']").fill(serial)
      await page.locator("input[name='imei']").fill(`35${String(Date.now()).slice(-13)}`)
      const uploadResponse = page.waitForResponse((response: any) => response.url().endsWith("/mobile-intake/photos") && response.request().method() === "POST")
      await page.locator('.photos-card input[type="file"]').first().setInputFiles({ name: "device.png", mimeType: "image/png", buffer: Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j7b8AAAAASUVORK5CYII=", "base64") })
      assert.equal((await uploadResponse).status(), 201)
      await page.locator(".van-uploader__preview-image img").waitFor()
      await page.locator(".van-uploader__preview-image").first().click()
      await page.locator(".van-image-preview").waitFor()
      assert(await page.locator(".van-image-preview img").count() > 0, "Uploaded photo opens in full-screen Vant preview")
      await page.locator(".van-image-preview__close-icon").click()
      await page.locator(".van-image-preview").waitFor({ state: "hidden" })
      await page.waitForFunction(() => !(document.querySelector("button[type='submit']") as HTMLButtonElement)?.disabled)
      const requestPromise = page.waitForResponse((response: any) => response.url() === `${base}/mobile-intake` && response.request().method() === "POST")
      await page.locator("button[type='submit']").click()
      const response = await requestPromise
      assert.equal(response.status(), 200, await response.text())
      await page.locator(".stats-trigger").click()
      await page.locator(".stats-device").waitFor()
      await page.locator(".stats-popup .popup-heading button").click()
      assert.equal(await page.locator("input[name='serial']").inputValue(), "", "Clears serial after receiving")
      const [device] = await service.listDevices({ serial_number: serial })
      assert.equal(device.source_id, source.id)
      assert.equal(device.state, "pending")
      assert.equal((device.photos as unknown as unknown[]).length, 1, "Phone photos are saved with the device")
      assert.equal(await page.locator(".van-uploader__preview-image img").count(), 0, "Photos cleared for the next device")
      const duplicate = await api("/mobile-intake", { method: "POST", headers, body: JSON.stringify({ variant_id: managed.variant_id, serial_number: serial, imei: device.imei }) })
      assert.equal(duplicate.status, 400, "Duplicate devices are rejected")
      await page.locator(".van-toast").waitFor({ state: "hidden" })
      await page.screenshot({ path: resolve(process.cwd(), `../../../logs/mobile-intake-${lang}.png`), fullPage: true })
      const scannedSerial = `SCANCHECK${Date.now()}`
      const label = await QRCode.toDataURL(`S/N:${scannedSerial};IMEI:123456789012345`, { width: 480, margin: 4 })
      await page.evaluate(async (dataUrl: string) => {
        const image = new Image()
        image.src = dataUrl
        await image.decode()
        const canvas = document.createElement("canvas")
        canvas.width = canvas.height = 640
        const drawing = canvas.getContext("2d")!
        const stream = canvas.captureStream(10)
        const draw = () => {
          if (stream.getVideoTracks()[0].readyState !== "live") return
          drawing.fillStyle = "white"
          drawing.fillRect(0, 0, 640, 640)
          drawing.drawImage(image, 80, 80, 480, 480)
          requestAnimationFrame(draw)
        }
        draw()
        navigator.mediaDevices.getUserMedia = async () => stream
      }, label)
      await page.getByRole("button", { name: lang === "zh" ? "打开摄像头扫码" : "Open camera scanner", exact: true }).click()
      await page.waitForFunction(() => Boolean((document.querySelector("video") as HTMLVideoElement)?.srcObject))
      await page.waitForFunction((serial: string) => (document.querySelector("input[name='serial']") as HTMLInputElement)?.value === serial, scannedSerial)
      assert.equal(await page.locator("input[name='imei']").inputValue(), "123456789012345", "Camera decodes both serial and IMEI from a real QR frame")
      await page.getByRole("button", { name: lang === "zh" ? "完成扫描" : "Done scanning", exact: true }).click()
      await page.waitForFunction(() => !(document.querySelector("video") as HTMLVideoElement)?.srcObject)
    }
    await page.goto(`${base}/static/mobile-intake/index.html#invalid`)
    await page.locator(".status-card").waitFor()
    if (process.env.MOBILE_INTAKE_SCAN_ONLY !== "1") {
    await page.route("**/admin/**", (route: any) => route.continue({ headers: { ...route.request().headers(), authorization: `Bearer ${admin}` } }))
    await page.goto(`${base}/app/device-scan/intake`)
    await page.waitForFunction(() => Boolean(document.querySelector("select")?.value))
    assert(await page.getByRole("button", { name: /Create temporary link and QR code|创建临时链接和二维码/ }).isEnabled(), "Owned warehouse is selected automatically and link creation is enabled")
    await page.locator("select").first().selectOption(source.id)
    await page.getByRole("button", { name: /Create temporary link and QR code|创建临时链接和二维码/ }).click()
    const qr = page.locator("img[alt='Mobile intake QR code'], img[alt='手机入库二维码']")
    await qr.waitFor()
    assert((await qr.getAttribute("src"))?.startsWith("data:image/png;base64,"), "QR generated locally")
    const mobileLink = page.locator("input[aria-label='Temporary intake link'], input[aria-label='临时入库链接']")
    assert((await mobileLink.inputValue()).includes("/static/mobile-intake/index.html?lang="), "Dashboard links to standalone HTML")
    }
    assert.deepEqual(errors, [], "No browser runtime errors")
    console.log("Mobile intake passed: scoped links, denied forged tokens, warehouse injection, real receiving in Chinese and English, duplicate rejection, phone layout, lazy barcode loading, QR frame decoding, camera start/stop, invalid-link state." + (process.env.MOBILE_INTAKE_SCAN_ONLY === "1" ? "" : " Dashboard link and QR generation passed."))
  } finally {
    await browser?.close()
    // Only isolated test products and warehouses are removed below.
    const devices = serials.length ? await service.listDevices({ serial_number: serials }) : []
    for (const device of devices) {
      const events = await service.listDeviceEvents({ device_id: device.id })
      if (events.length) await service.deleteDeviceEvents(events.map((event) => event.id))
      const photos = device.photos as unknown as { id: string }[]
      if (photos.length) await container.resolve(Modules.FILE).deleteFiles(photos.map((photo) => photo.id))
      await service.softDeleteDevices(device.id)
    }
    if (managed!) {
      const events = (await service.listDeviceEvents({}, { take: 10000 })).filter((event) => event.data.variant_id === managed.variant_id)
      if (events.length) await service.deleteDeviceEvents(events.map((event) => event.id))
      const enrolled = await service.listDeviceVariants({ variant_id: managed.variant_id })
      if (enrolled.length) await service.deleteDeviceVariants(enrolled.map((entry) => entry.id))
    }
    if (productId) await deleteProductsWorkflow(container).run({ input: { ids: [productId] } })
    if (source!) {
      await service.deleteDeviceSources(source.id)
      await deleteStockLocationsWorkflow(container).run({ input: { ids: [source.location_id] } })
    }
  }
}
