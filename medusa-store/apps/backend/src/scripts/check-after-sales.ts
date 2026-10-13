import assert from "node:assert/strict"
import { resolve } from "node:path"
import type { ConfigModule, ExecArgs, IOrderModuleService, ICustomerModuleService, IApiKeyModuleService, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, generateJwtToken, Modules } from "@medusajs/framework/utils"
import { AFTER_SALES_MODULE } from "../modules/after-sales"
import AfterSalesModuleService from "../modules/after-sales/service"

// Creates isolated fixtures and removes them in finally; no real order is changed.
export default async function checkAfterSales({ container }: ExecArgs) {
  const orders: IOrderModuleService = container.resolve(Modules.ORDER)
  const customers: ICustomerModuleService = container.resolve(Modules.CUSTOMER)
  const keys: IApiKeyModuleService = container.resolve(Modules.API_KEY)
  const users: IUserModuleService = container.resolve(Modules.USER)
  const service: AfterSalesModuleService = container.resolve(AFTER_SALES_MODULE)
  const config: ConfigModule = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const [key] = await keys.listApiKeys({ type: "publishable" })
  const [user] = await users.listUsers({})
  assert(key && user, "A publishable key and admin user are required")
  const customer = await customers.createCustomers({ email: `after-sales-check-${Date.now()}@example.invalid` })
  let orderId: string | undefined
  const token = (actor_id: string, actor_type = "customer") => generateJwtToken({
    actor_id, actor_type, auth_identity_id: "after_sales_smoke_test",
    app_metadata: { [actor_type === "customer" ? "customer_id" : "user_id"]: actor_id },
  }, { secret: config.projectConfig.http.jwtSecret, expiresIn: "5m" })
  const base = "http://127.0.0.1:9001"
  const call = (path: string, auth?: string, body?: unknown) => fetch(`${base}${path}`, {
    method: body ? "POST" : "GET",
    headers: { "x-publishable-api-key": key.token, "Content-Type": "application/json", ...(auth ? { authorization: `Bearer ${auth}` } : {}) },
    ...(body ? { body: JSON.stringify(body) } : {}),
  })
  try {
    const order = await orders.createOrders({ currency_code: "eur", customer_id: customer.id, email: customer.email!,
      items: [{ title: "After-sales smoke test", quantity: 2, unit_price: 100 }] })
    orderId = order.id
    const path = `/store/orders/${order.id}/after-sales`
    const adminPath = `/admin/orders/${order.id}/after-sales`
    const auth = token(customer.id)
    const adminAuth = token(user.id, "user")
    const itemId = order.items![0].id
    const body = { type: "repair", reason: "The test device does not turn on.", items: [{ item_id: itemId, quantity: 1 }] }
    assert.equal((await call(path)).status, 401, "Anonymous read rejected")
    assert.equal((await call(path, undefined, body)).status, 401, "Anonymous submission rejected")
    assert.equal((await call(path, token("other_customer"))).status, 404, "Other customer cannot read")
    assert.equal((await call(path, token("other_customer"), body)).status, 404, "Other customer cannot submit")
    assert.equal((await call(path, auth, { ...body, reason: "short" })).status, 400, "Reason validated")
    assert.equal((await call(path, auth, { ...body, items: [{ item_id: itemId, quantity: 3 }] })).status, 400, "Quantity validated")
    assert.equal((await call(path, auth, { ...body, items: [{ item_id: "unrelated_item", quantity: 1 }] })).status, 400, "Item ownership validated")
    assert.equal((await call(path, auth, { ...body, items: [...body.items, ...body.items] })).status, 400, "Duplicate items rejected")
    const concurrent = await Promise.all([call(path, auth, body), call(path, auth, body)])
    assert.deepEqual(concurrent.map((response) => response.status).sort(), [201, 400], "Only one concurrent request accepted")
    const listing = await (await call(path, auth)).json() as { requests: { id: string; status: string; items: { quantity: number }[] }[] }
    assert.equal(listing.requests.length, 1)
    assert.equal(listing.requests[0].status, "pending")
    assert.equal(listing.requests[0].items[0].quantity, 1)
    const requestId = listing.requests[0].id
    assert.equal((await call(adminPath)).status, 401, "Admin authentication required")
    assert.equal((await call(adminPath, auth)).status, 401, "Customer cannot access admin")
    assert.equal((await call(adminPath, adminAuth)).status, 200)
    assert.equal((await call(adminPath, adminAuth, { request_id: requestId, status: "processing" })).status, 200)
    const processing = await (await call(path, auth)).json() as { requests: { status: string }[] }
    assert.equal(processing.requests[0].status, "processing", "Customer sees admin update")
    assert.equal((await call(adminPath, adminAuth, { request_id: requestId, status: "completed" })).status, 200)
    assert.equal((await call(adminPath, adminAuth, { request_id: requestId, status: "processing" })).status, 400, "Closed request cannot reopen")
    const second = await call(path, auth, body)
    assert.equal(second.status, 201, "A new request is allowed after the previous one closes")
    const secondRequest = await second.json() as { request: { id: string } }
    if (process.env.PLAYWRIGHT_MODULE_PATH) {
      assert.equal((await call(adminPath, adminAuth, { request_id: secondRequest.request.id, status: "rejected" })).status, 200)
      const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
      const browser = await chromium.launch({ channel: "chrome", headless: true })
      try {
        const context = await browser.newContext({ viewport: { width: 1280, height: 1000 } })
        const page = await context.newPage()
        await page.goto(`http://127.0.0.1:8000/dk/order/${order.id}/confirmed`)
        await page.getByRole("button", { name: "Request after-sales support" }).click()
        await page.getByTestId("after-sales-section").getByRole("link", { name: "sign in" }).waitFor()
        await context.addCookies([{ name: "_medusa_jwt", value: auth, domain: "127.0.0.1", path: "/" }])
        await page.goto(`http://127.0.0.1:8000/dk/account/orders/details/${order.id}`)
        await page.getByRole("button", { name: "Request after-sales support" }).click()
        await page.getByLabel("Request type").selectOption("repair")
        await page.getByRole("checkbox").check()
        await page.getByLabel("Describe the issue").fill("The battery is not charging after delivery.")
        await page.getByTestId("after-sales-section").scrollIntoViewIfNeeded()
        await page.screenshot({ path: resolve(process.cwd(), "../../../logs/after-sales-form-desktop.png"), fullPage: true })
        await page.setViewportSize({ width: 390, height: 844 })
        await page.screenshot({ path: resolve(process.cwd(), "../../../logs/after-sales-form-mobile.png"), fullPage: true })
        assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), "No horizontal overflow on mobile")
        await page.getByRole("button", { name: "Submit request", exact: true }).click()
        await page.getByText("Your request has been submitted.", { exact: false }).waitFor()
        await page.reload()
        await page.getByRole("button", { name: "Request after-sales support" }).click()
        await page.getByText("Awaiting review", { exact: true }).waitFor()
        console.log("After-sales browser checks passed: guest sign-in, desktop/mobile form, submission and persisted status.")
      } finally { await browser.close() }
    }
    console.log("After-sales HTTP checks passed: authentication, ownership, validation, concurrent submission, persistence and status updates.")
  } finally {
    if (orderId) {
      const requests = await service.listAfterSalesRequests({ order_id: orderId })
      if (requests.length) await service.deleteAfterSalesRequests(requests.map((request) => request.id))
      await orders.deleteOrders(orderId)
    }
    await customers.deleteCustomers(customer.id)
  }
}
