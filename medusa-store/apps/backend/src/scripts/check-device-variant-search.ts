import assert from "node:assert/strict"
import { resolve } from "node:path"
import type { ConfigModule, ExecArgs, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"
import { deviceVariantSearchFilters } from "../lib/device-variant-search"

export default async function checkDeviceVariantSearch({ container }: ExecArgs) {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const matchedIds = new Map<string, string[]>()
  for (const search of ["i14pm", "i14pm 128", "i14pm 128 85 s", "i14pm 128 85 s 黑", "黑 s 85 128 i14pm", "iPhone 14 Pro Max 128GB 85% S级 Black", "iphone14pro max 128 85 s 黑", "i14pm 1t", "ERP10001", "no-such-variant-xyz"]) {
    const { data, metadata } = await query.graph({
      entity: "product_variant",
      fields: ["id", "sku", "title", "product.title", "options.value"],
      filters: deviceVariantSearchFilters(search),
      pagination: { take: 20, skip: 0 },
    })
    console.log(JSON.stringify({ search, count: metadata?.count, sample: data[0] }))
    matchedIds.set(search, data.map((variant) => variant.id).sort())
    if (search.startsWith("i14pm")) assert(data.length > 0, `Expected results for ${search}`)
    if (search.includes("128")) {
      for (const variant of data) {
        assert.equal(variant.product?.title, "iPhone 14 Pro Max")
        assert(variant.options?.some((option) => option.value === "128 GB"))
        if (search.includes("85")) assert(variant.options?.some((option) => option.value === "85%"))
        if (search.toLowerCase().includes(" s")) assert(variant.options?.some((option) => option.value === "S"))
      }
    }
    if (search === "i14pm 1t") assert(data.every((variant) => variant.options?.some((option) => option.value === "1024 GB")))
    if (search === "no-such-variant-xyz") assert.equal(data.length, 0)
  }
  assert.deepEqual(matchedIds.get("i14pm 128 85 s 黑"), matchedIds.get("黑 s 85 128 i14pm"))
  assert.deepEqual(matchedIds.get("i14pm 128 85 s 黑"), matchedIds.get("iPhone 14 Pro Max 128GB 85% S级 Black"))
  assert.deepEqual(matchedIds.get("i14pm 128 85 s 黑"), matchedIds.get("iphone14pro max 128 85 s 黑"))

  if (!process.env.PLAYWRIGHT_MODULE_PATH) return
  const config: ConfigModule = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const users: IUserModuleService = container.resolve(Modules.USER)
  const [user] = await users.listUsers({})
  assert(user, "An admin user is required for browser checks")
  const token = generateJwtToken({
    actor_id: user.id, actor_type: "user", auth_identity_id: "variant_search_check",
    app_metadata: { user_id: user.id },
  }, { secret: config.projectConfig.http.jwtSecret, expiresIn: "5m" })
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
  const browser = await chromium.launch({ channel: "chrome", headless: true })
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } })
    await context.addCookies([{ name: "lng", value: "zhCN", url: "http://127.0.0.1:9001" }])
    const page = await context.newPage()
    const errors: string[] = []
    page.on("pageerror", (error: Error) => errors.push(error.message))
    await page.route("**/admin/**", async (route: any) => {
      if (["fetch", "xhr"].includes(route.request().resourceType())) {
        await route.continue({ headers: { ...route.request().headers(), authorization: `Bearer ${token}` } })
      } else await route.continue()
    })
    for (let attempt = 0; attempt < 25; attempt++) {
      try {
        if ((await context.request.get("http://127.0.0.1:9001/health", { timeout: 1000 })).ok()) break
      } catch { /* The development server may be restarting after a source edit. */ }
      await new Promise((done) => setTimeout(done, 1000))
    }
    const api = await context.request.get("http://127.0.0.1:9001/admin/devices/variants", {
      headers: { authorization: `Bearer ${token}` }, params: { q: "i14pm 128 85 s" },
    })
    assert.equal(api.status(), 200, await api.text())
    assert.equal((await api.json()).count, matchedIds.get("i14pm 128 85 s")?.length)
    await page.goto("http://127.0.0.1:9001/app/device-scan")
    await page.getByTestId("device-scan-page").waitFor()
    assert.equal(new URL(page.url()).pathname, "/app/device-scan/lookup")
    const pages = [
      ["查设备", "lookup"], ["设备入库", "intake"], ["配货出库", "orders"],
      ["退回检测", "inspection"], ["仓库与供应商", "sources"],
    ]
    for (const [label, path] of pages) {
      await page.getByRole("link", { name: label, exact: true }).click()
      await page.getByRole("heading", { name: label, exact: true }).waitFor()
      await page.getByTestId("device-scan-page").locator("fieldset").waitFor()
      assert.equal(new URL(page.url()).pathname, `/app/device-scan/${path}`)
      assert.equal(await page.getByRole("button", { name: label, exact: true }).count(), 0)
      const fields = page.getByTestId("device-scan-page").locator('input:not([type="file"]):not([type="checkbox"]), textarea')
      for (let index = 0; index < await fields.count(); index++) {
        const field = fields.nth(index)
        if (!await field.isVisible() || !await field.isEditable()) continue
        await field.fill(await field.getAttribute("type") === "number" ? "12" : "CLEAR-CHECK")
        const clear = field.locator('xpath=ancestor::div[button[@aria-label="清空输入"]][1]').getByRole("button", { name: "清空输入", exact: true })
        await clear.click()
        assert.equal(await field.inputValue(), "")
        await clear.waitFor({ state: "hidden" })
        assert(await field.evaluate((element: HTMLInputElement) => element === document.activeElement), "Clear keeps focus on the input")
      }
    }
    await page.reload()
    await page.getByRole("heading", { name: "仓库与供应商", exact: true }).waitFor()
    await page.getByText("启用 SKU 设备库存", { exact: true }).waitFor()
    await page.screenshot({ path: resolve(process.cwd(), "../../../logs/device-directory-desktop.png") })
    await page.goto("http://127.0.0.1:9001/app/device-scan?order_id=order_navigation_check")
    await page.getByRole("heading", { name: "配货出库", exact: true }).waitFor()
    assert.equal(new URL(page.url()).pathname, "/app/device-scan/orders")
    assert.equal(new URL(page.url()).searchParams.get("order_id"), "order_navigation_check")
    await page.getByRole("link", { name: "设备入库", exact: true }).click()
    await page.getByRole("heading", { name: "设备入库", exact: true }).waitFor()
    const trigger = page.getByRole("button", { name: "商品 SKU", exact: true })
    await trigger.click()
    const input = page.getByRole("combobox", { name: "搜索商品 SKU" })
    const options = page.getByRole("listbox", { name: "商品 SKU" }).getByRole("option")
    await input.fill("clear-check")
    await input.locator('xpath=ancestor::div[button[@aria-label="清空输入"]][1]').getByRole("button", { name: "清空输入", exact: true }).click()
    assert.equal(await input.inputValue(), "")
    await input.fill("i14pm 128 85 s")
    await page.getByText("找到 4 个 SKU", { exact: true }).waitFor()
    assert.equal(await options.count(), 4)
    await input.fill("i14pm 128 85 s 黑")
    await page.getByText("找到 1 个 SKU", { exact: true }).waitFor()
    const choice = options
    assert((await choice.innerText()).includes("Space Black"))
    await page.screenshot({ path: resolve(process.cwd(), "../../../logs/sku-search-desktop.png") })
    const refresh = page.waitForResponse((response: any) => response.url().includes("selected_variant_id=") && response.status() === 200)
    await input.press("ArrowDown")
    await input.press("Enter")
    await page.getByRole("dialog").waitFor({ state: "hidden" })
    assert((await trigger.innerText()).includes("Space Black"))
    // A selected result outside the old 200-row list survives the page's refresh.
    await refresh
    assert((await trigger.innerText()).includes("Space Black"))
    await page.setViewportSize({ width: 390, height: 844 })
    await trigger.click()
    await input.fill("i14pm 128 85 s 黑")
    await page.getByText("找到 1 个 SKU", { exact: true }).waitFor()
    await page.waitForFunction(() => {
      const dialog = document.querySelector('[role="dialog"]')
      return dialog && getComputedStyle(dialog).opacity === "1"
    })
    await page.screenshot({ path: resolve(process.cwd(), "../../../logs/sku-search-mobile.png") })
    const bounds = await page.getByRole("dialog").boundingBox()
    assert(bounds && bounds.x >= 0 && bounds.x + bounds.width <= 391, "Picker fits the mobile viewport")
    await input.fill("no-such-variant-xyz")
    await page.getByText("没有匹配的 SKU，请减少或修改搜索条件", { exact: true }).waitFor()
    await input.fill("")
    await options.first().waitFor()
    const firstPage = await options.first().innerText()
    await page.getByRole("dialog").getByRole("button", { name: "下一页", exact: true }).click()
    await options.first().waitFor()
    assert.notEqual(await options.first().innerText(), firstPage)
    await page.getByRole("button", { name: "清除选择", exact: true }).click()
    assert.equal((await trigger.innerText()).trim(), "请选择")
    const deviceData = await context.request.get("http://127.0.0.1:9001/admin/devices", {
      headers: { authorization: `Bearer ${token}` },
    })
    assert.equal(deviceData.status(), 200)
    const ownSource = (await deviceData.json()).sources.find((source: any) => source.type === "self")
    assert(ownSource, "An own warehouse is required for intake checks")
    await page.getByLabel("供货来源 / 收回仓库", { exact: true }).selectOption(ownSource.id)
    await trigger.click()
    await input.fill("ERP11503")
    await page.getByText("找到 1 个 SKU", { exact: true }).waitFor()
    await options.click()
    await page.getByRole("textbox", { name: "苹果序列号", exact: true }).fill("SKU-INTAKE-CHECK")
    const intakeButton = page.getByRole("button", { name: "登记入库", exact: true })
    assert(await intakeButton.isDisabled(), "iPhone intake requires an IMEI")
    await page.getByText("iPhone 入库需要至少填写一个 IMEI（15 位）或 EID（32 位）。", { exact: true }).waitFor()
    const imeiField = page.getByRole("textbox", { name: "IMEI / EID 1", exact: true })
    const invalidImei = "12312321311888899944"
    await imeiField.fill(invalidImei)
    assert(await intakeButton.isDisabled(), "An overlong IMEI cannot be submitted")
    await page.getByRole("alert").filter({ hasText: `请输入 15 位 IMEI 或 32 位 EID（纯数字），当前输入 ${invalidImei.length} 位。` }).waitFor()
    assert.equal(await imeiField.getAttribute("aria-invalid"), "true")
    await page.screenshot({ path: resolve(process.cwd(), "../../../logs/intake-imei-validation-mobile.png"), fullPage: true })
    await imeiField.fill("12345678912345x")
    assert(await intakeButton.isDisabled(), "IMEI must contain digits only")
    await page.getByRole("textbox", { name: "IMEI / EID 1", exact: true }).fill("123456789123456")
    assert(await intakeButton.isEnabled(), "A normal SKU can be registered without legacy device enrollment")
    assert.equal(await imeiField.getAttribute("aria-invalid"), "false")
    const secondIdentity = page.getByRole("textbox", { name: "IMEI / EID 2", exact: true })
    const eid = "89049032000000000000000000000001"
    await secondIdentity.fill("123456789123456")
    assert(await intakeButton.isDisabled(), "Duplicate identifiers are rejected")
    await secondIdentity.fill(eid)
    assert(await intakeButton.isEnabled(), "IMEI plus EID is accepted")
    await imeiField.fill("")
    assert(await intakeButton.isEnabled(), "EID in the second slot alone is accepted")
    await imeiField.fill(eid)
    await secondIdentity.fill("")
    assert(await intakeButton.isEnabled(), "EID in the first slot alone is accepted")
    await imeiField.fill("123456789123456")
    await secondIdentity.fill(eid)
    await page.screenshot({ path: resolve(process.cwd(), "../../../logs/device-dual-identity-mobile.png"), fullPage: true })
    await page.setViewportSize({ width: 1440, height: 1000 })
    await page.screenshot({ path: resolve(process.cwd(), "../../../logs/device-dual-identity-desktop.png"), fullPage: true })

    assert.equal(await page.getByLabel("成色", { exact: true }).count(), 0)
    assert.equal(await page.getByLabel("实际电池健康度", { exact: true }).count(), 0)
    let intakePayload: any
    await page.route("**/admin/devices", async (route: any) => {
      if (route.request().method() === "POST") {
        intakePayload = route.request().postDataJSON()
        // Inspect the request without creating a device or changing real stock.
        await route.fulfill({ status: 400, json: { message: "Intake request checked without saving" } })
      } else await route.fallback()
    })
    await intakeButton.click()
    await page.getByText("Intake request checked without saving", { exact: true }).waitFor()
    assert.equal(intakePayload.action, "intake")
    assert.equal(intakePayload.devices[0].source_id, ownSource.id)
    assert.equal(intakePayload.devices[0].serial_number, "SKU-INTAKE-CHECK")
    assert.equal(intakePayload.devices[0].imei, "123456789123456")
    assert.equal(intakePayload.devices[0].imei2, eid)
    assert.equal(intakePayload.devices[0].grade, undefined)
    assert.equal(intakePayload.devices[0].battery_health, undefined)
    assert.deepEqual(errors, [], "No browser runtime errors")
    console.log("Browser checks passed: sidebar pages, clearable fields, SKU search, both IMEI/EID slots, either slot alone, duplicate/invalid identity feedback, mobile layout and intake payload (no stock changes)")
  } finally {
    await browser.close()
  }
}
