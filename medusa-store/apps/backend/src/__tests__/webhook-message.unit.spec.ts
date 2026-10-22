import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { AFTER_SALES_MODULE } from "../modules/after-sales"
import { formatWebhookMessage, loadWebhookContext, webhookAdminLink, webhookMessageText } from "../modules/webhook-notifications/message"
import { buildRequest } from "../modules/webhook-notifications/transport"
import { webhookEvents } from "../lib/webhook-events"

const order = {
  id: "order_test", custom_display_id: "202610220001", display_id: 12,
  email: "buyer@example.com", created_at: "2026-10-22T08:30:00Z", currency_code: "eur", total: 499.99,
  status: "pending", payment_status: "partially_captured", fulfillment_status: "not_fulfilled",
  shipping_address: { first_name: "Test", last_name: "Buyer", address_1: "Example street 1", address_2: "Unit 2", city: "Berlin", postal_code: "10115", country_code: "de" },
  payment_collections: [{ captured_amount: 200, refunded_amount: 50 }],
  items: [{ id: "item_test", title: "iPhone 15", variant_title: "128GB / Black", variant_sku: "IP15-128-BLK", quantity: 2, unit_price: 249.995 }],
  fulfillments: [{ id: "ful_test", labels: [{ tracking_number: "TRACK123", tracking_url: "https://carrier.example.com/track/123" }] }],
}
const payment = { id: "pay_test", payment_collection_id: "paycol_test", currency_code: "eur", amount: 499.99, captures: [{ amount: 200 }], refunds: [{ amount: 50 }] }
const afterSales = { id: "asr_test", order_id: order.id, type: "return", status: "processing", reason: "Wrong configuration\nPlease review", created_at: "2026-10-22T09:00:00Z", items: { lines: [{ item_id: "item_test", quantity: 1 }] } }
const originalAdmin = process.env.WEBHOOK_ADMIN_URL
const originalTimezone = process.env.WEBHOOK_TIME_ZONE
beforeEach(() => { process.env.WEBHOOK_ADMIN_URL = "https://admin.example.com/app"; process.env.WEBHOOK_TIME_ZONE = "Asia/Shanghai" })
afterAll(() => {
  if (originalAdmin === undefined) delete process.env.WEBHOOK_ADMIN_URL
  else process.env.WEBHOOK_ADMIN_URL = originalAdmin
  if (originalTimezone === undefined) delete process.env.WEBHOOK_TIME_ZONE
  else process.env.WEBHOOK_TIME_ZONE = originalTimezone
})

it.each(["zh", "en"] as const)("includes order details, exact amounts and an authenticated Admin link in %s", language => {
  const message = formatWebhookMessage({ resourceId: order.id, order }, "order.placed", language, "Operations")
  const text = webhookMessageText(message)
  for (const value of ["202610220001", "buyer@example.com", "Example street 1", "Unit 2", "Berlin", "10115", "IP15-128-BLK", "499.99", "200.00", "50.00", "16:30", "Operations"]) expect(text).toContain(value)
  expect(message.lines[0]).toBe("Operations")
  expect(message.links[0].url).toBe("https://admin.example.com/app/orders/order_test")
  expect(text).not.toContain("4.99")
  expect(text).not.toContain("undefined")
})

it("distinguishes payment amount from cumulative captured and refunded amounts", () => {
  const text = webhookMessageText(formatWebhookMessage({ resourceId: payment.id, order, payment }, "payment.refunded", "zh"))
  expect(text).toContain("支付单金额: €499.99")
  expect(text).toContain("本支付累计实收: €200.00")
  expect(text).toContain("本支付累计退款: €50.00")
})

it.each(["return", "exchange", "repair"])("includes %s after-sales details and the affected SKU and quantity", type => {
  const text = webhookMessageText(formatWebhookMessage({ resourceId: afterSales.id, order, afterSales: { ...afterSales, type } }, "after_sales.processing", "en"))
  expect(text).toContain("asr_test")
  expect(text).toContain("Wrong configuration Please review")
  expect(text).toContain("SKU: IP15-128-BLK × 1")
  expect(text).toContain("Request status: Processing")
  expect(text).toContain("/app/orders/order_test")
})

it("includes shipment tracking and rejects executable carrier URLs", () => {
  const context = { resourceId: "ful_test", order }
  expect(formatWebhookMessage(context, "shipment.created", "zh").links[1].url).toBe("https://carrier.example.com/track/123")
  const unsafe = { ...order, fulfillments: [{ id: "ful_test", labels: [{ tracking_url: "javascript:alert(1)" }] }] }
  expect(formatWebhookMessage({ ...context, order: unsafe }, "shipment.created", "zh").links).toHaveLength(1)
})

it.each(["https://admin.example.com", "https://admin.example.com/app/", "https://admin.example.com/app?x=1#section"])("normalizes configured Admin URL %s", url => {
  process.env.WEBHOOK_ADMIN_URL = url
  expect(webhookAdminLink(order.id)).toBe("https://admin.example.com/app/orders/order_test")
})

it("builds a Feishu rich post with a native clickable link, safe text and signing", () => {
  const message = formatWebhookMessage({ resourceId: order.id, order: { ...order, email: "<at user_id=all>test</at>" } }, "order.placed", "zh", "安全关键词")
  const { body } = buildRequest({ provider: "feishu", url: "https://open.feishu.cn/open-apis/bot/v2/hook/test", secret: "secret", token: "" }, webhookMessageText(message), "order.placed", order.id, 1700000000123, message)
  expect(body.msg_type).toBe("post")
  expect(body.sign).toBeTruthy()
  const content = (body.content as any).post.zh_cn.content
  expect(content[0]).toEqual([{ tag: "text", text: "安全关键词" }])
  expect(content.flat().some(node => node.tag === "at")).toBe(false)
  expect(content.flat()).toContainEqual({ tag: "a", text: "查看订单 / 处理售后", href: "https://admin.example.com/app/orders/order_test" })
})

it("keeps long Feishu messages below the platform limit while preserving the action link", () => {
  const message = { title: "Test", language: "zh" as const, lines: Array.from({ length: 50 }, () => "商品".repeat(300)), links: [{ label: "查看订单", url: "https://admin.example.com/app/orders/order_test" }] }
  const { body } = buildRequest({ provider: "feishu", url: "https://open.feishu.cn/open-apis/bot/v2/hook/test", secret: "", token: "" }, webhookMessageText(message), "order.placed", order.id, 0, message)
  expect(Buffer.byteLength(JSON.stringify(body))).toBeLessThan(20000)
  expect(JSON.stringify(body)).toContain(message.links[0].url)
})

it("keeps long WeCom messages within its text limit and preserves the order link", () => {
  const message = { title: "Test", language: "zh" as const, lines: Array.from({ length: 50 }, () => "商品".repeat(300)), links: [{ label: "查看订单", url: "https://admin.example.com/app/orders/order_test" }] }
  const { body } = buildRequest({ provider: "wecom", url: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test", secret: "", token: "" }, webhookMessageText(message), "order.placed", order.id, 0, message)
  expect(Buffer.byteLength((body.text as any).content)).toBeLessThanOrEqual(2048)
  expect((body.text as any).content).toContain(message.links[0].url)
})

it.each(webhookEvents)("renders the subscribed event %s in both languages", event => {
  for (const language of ["zh", "en"] as const) {
    const text = webhookMessageText(formatWebhookMessage({ resourceId: "test", order }, event, language))
    expect(text).not.toContain("undefined")
    expect(text).toContain("/app/orders/order_test")
  }
})

describe("business context resolution", () => {
  function setup() {
    const graph = jest.fn(async ({ entity }) => ({ data: entity === "payment" ? [payment] : entity === "order" ? [order] : [{ order: { id: order.id } }] }))
    const retrieve = jest.fn().mockResolvedValue(afterSales)
    const container = { resolve: (key: string) => key === ContainerRegistrationKeys.QUERY ? { graph } : key === AFTER_SALES_MODULE ? { retrieveAfterSalesRequest: retrieve } : undefined } as any
    return { graph, retrieve, container }
  }
  it.each(["payment.captured", "payment.refunded"] as const)("resolves the order associated with %s", async event => {
    const { container, graph } = setup()
    const context = await loadWebhookContext(container, event, { id: payment.id })
    expect(context.order?.id).toBe(order.id)
    expect(context.payment?.amount).toBe(499.99)
    expect(graph).toHaveBeenCalledWith(expect.objectContaining({ entity: "order_payment_collection", filters: { payment_collection_id: payment.payment_collection_id } }))
  })
  it.each(["shipment.created", "delivery.created"] as const)("resolves the order from the %s fulfillment ID", async event => {
    const { container, graph } = setup()
    expect((await loadWebhookContext(container, event, { id: "ful_test" })).order?.id).toBe(order.id)
    expect(graph).toHaveBeenCalledWith(expect.objectContaining({ entity: "order_fulfillment", filters: { fulfillment_id: "ful_test" } }))
  })
  it("loads the full after-sales request without depending on its event payload for customer data", async () => {
    const { container, retrieve } = setup()
    const context = await loadWebhookContext(container, "after_sales.created", { id: afterSales.id })
    expect(retrieve).toHaveBeenCalledWith(afterSales.id)
    expect(context.order?.id).toBe(order.id)
    expect(context.afterSales?.reason).toBe(afterSales.reason)
    expect(context.afterSales?.status).toBe("pending")
  })
  it("shows the status represented by a queued event even if the request is already completed", async () => {
    const { container, retrieve } = setup()
    retrieve.mockResolvedValue({ ...afterSales, status: "completed" })
    const context = await loadWebhookContext(container, "after_sales.processing", { id: afterSales.id })
    expect(context.afterSales?.status).toBe("processing")
  })
  it.each([
    ["order.fulfillment_created", "fulfillment_id"], ["order.fulfillment_canceled", "fulfillment_id"],
    ["order.return_requested", "return_id"], ["order.return_received", "return_id"],
    ["order.exchange_created", "exchange_id"], ["order.claim_created", "claim_id"],
  ] as const)("uses the native %s resource field %s", async (event, field) => {
    const { container } = setup()
    const context = await loadWebhookContext(container, event, { order_id: order.id, [field]: "resource_test" })
    expect(context.resourceId).toBe("resource_test")
    expect(context.order?.id).toBe(order.id)
  })
  it("does not fabricate order data for a payment that has no order yet", async () => {
    const { container, graph } = setup()
    graph.mockImplementation(async ({ entity }) => ({ data: entity === "payment" ? [payment] : [] } as any))
    const context = await loadWebhookContext(container, "payment.captured", { id: payment.id })
    expect(context.order).toBeUndefined()
    expect(webhookMessageText(formatWebhookMessage(context, "payment.captured", "en"))).toContain("Payment amount: €499.99")
  })
})
