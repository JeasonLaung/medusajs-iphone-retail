import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import subscriber, { config } from "../subscribers/webhook-notifications"
import { WEBHOOK_NOTIFICATIONS_MODULE } from "../modules/webhook-notifications"
import { deliver } from "../modules/webhook-notifications/transport"
import { loadWebhookContext } from "../modules/webhook-notifications/message"
import { webhookEvents } from "../lib/webhook-events"

jest.mock("../modules/webhook-notifications/transport", () => ({ ...jest.requireActual("../modules/webhook-notifications/transport"), deliver: jest.fn() }))
jest.mock("../modules/webhook-notifications/message", () => ({ ...jest.requireActual("../modules/webhook-notifications/message"), loadWebhookContext: jest.fn() }))
jest.mock("../modules/webhook-notifications/security", () => ({ decrypt: (value: string) => `decrypted:${value}` }))

const endpoint = { id: "whk_test", provider: "feishu", enabled: true, events: { values: [...webhookEvents] }, language: "zh", keyword: "安全关键词", url: "encrypted-url", secret: "encrypted-secret", token: "" }
const resources = { fulfillment_id: "ful_test", return_id: "return_test", exchange_id: "exchange_test", claim_id: "claim_test" }
function setup(endpoints: any[] = [endpoint]) {
  const service = { listWebhookEndpoints: jest.fn().mockResolvedValue(endpoints), updateWebhookEndpoints: jest.fn().mockResolvedValue({}) }
  const logger = { warn: jest.fn() }
  const container = { resolve: jest.fn((key: string) => key === WEBHOOK_NOTIFICATIONS_MODULE ? service : key === ContainerRegistrationKeys.LOGGER ? logger : undefined) }
  return { service, logger, container }
}
beforeEach(() => {
  jest.clearAllMocks()
  jest.mocked(deliver).mockResolvedValue(undefined)
  jest.mocked(loadWebhookContext).mockImplementation(async (_container, _name, data) => ({
    resourceId: data.id || data.fulfillment_id || data.return_id || data.exchange_id || data.claim_id!,
    order: { id: "order_test", display_id: 12, email: "buyer@example.com", currency_code: "eur", total: 499.99, items: [{ id: "item_test", variant_sku: "PHONE-SKU", quantity: 1 }] },
  }))
})

it("registers every event supported by the Admin and API", () => expect(config.event).toEqual([...webhookEvents]))

it.each(webhookEvents)("delivers %s as a rich business notification", async name => {
  const { container, service } = setup()
  const resourceField = name.includes("fulfillment_") ? "fulfillment_id" : name.includes("return_") ? "return_id" : name === "order.exchange_created" ? "exchange_id" : name === "order.claim_created" ? "claim_id" : null
  const data = resourceField ? { order_id: "order_test", [resourceField]: resources[resourceField] } : { id: "resource_test" }
  await subscriber({ event: { name, data }, container } as any)
  expect(service.listWebhookEndpoints).toHaveBeenCalledWith({ enabled: true })
  expect(deliver).toHaveBeenCalledTimes(1)
  const [target, text, sentEvent, id, message] = jest.mocked(deliver).mock.calls[0]
  expect(target.url).toBe("decrypted:encrypted-url")
  expect(sentEvent).toBe(name)
  expect(id).toBe(resourceField ? resources[resourceField] : "resource_test")
  expect(text).toContain("buyer@example.com")
  expect(text).toContain("PHONE-SKU")
  expect(message?.links[0].url).toContain("/app/orders/order_test")
  expect(service.updateWebhookEndpoints).toHaveBeenCalledWith(expect.objectContaining({ id: endpoint.id, last_status: "success" }))
})

it("does not query business details or send to disabled and unsubscribed channels", async () => {
  const { container } = setup([{ ...endpoint, enabled: false }, { ...endpoint, id: "other", events: { values: ["order.canceled"] } }])
  await subscriber({ event: { name: "order.placed", data: { id: "order_test" } }, container } as any)
  expect(loadWebhookContext).not.toHaveBeenCalled()
  expect(deliver).not.toHaveBeenCalled()
})

it("loads business details once while formatting each channel's language", async () => {
  const { container } = setup([endpoint, { ...endpoint, id: "whk_en", language: "en" }])
  await subscriber({ event: { name: "order.placed", data: { id: "order_test" } }, container } as any)
  expect(loadWebhookContext).toHaveBeenCalledTimes(1)
  expect(jest.mocked(deliver).mock.calls[0][4]?.title).toContain("新订单")
  expect(jest.mocked(deliver).mock.calls[1][4]?.title).toContain("Order placed")
})

it("records a failed channel and continues delivering to other channels", async () => {
  jest.mocked(deliver).mockRejectedValueOnce(new Error("Network failure"))
  const { container, service, logger } = setup([endpoint, { ...endpoint, id: "whk_second" }])
  await expect(subscriber({ event: { name: "payment.refunded", data: { id: "pay_test" } }, container } as any)).resolves.toBeUndefined()
  expect(deliver).toHaveBeenCalledTimes(2)
  expect(service.updateWebhookEndpoints).toHaveBeenCalledWith(expect.objectContaining({ id: endpoint.id, last_status: "failed" }))
  expect(service.updateWebhookEndpoints).toHaveBeenCalledWith(expect.objectContaining({ id: "whk_second", last_status: "success" }))
  expect(JSON.stringify(logger.warn.mock.calls)).not.toContain("buyer@example.com")
})

it("records a data-loading failure instead of sending incomplete order details", async () => {
  jest.mocked(loadWebhookContext).mockRejectedValueOnce(new Error("Order unavailable"))
  const { container, service } = setup()
  await subscriber({ event: { name: "order.placed", data: { id: "order_test" } }, container } as any)
  expect(deliver).not.toHaveBeenCalled()
  expect(service.updateWebhookEndpoints).toHaveBeenCalledWith(expect.objectContaining({ last_status: "failed" }))
})

it.each([{ name: "unknown.event", data: { id: "test" } }, { name: "order.return_received", data: { order_id: "order_test" } }])("ignores unsupported or incomplete events %j", async event => {
  const { container } = setup()
  await subscriber({ event, container } as any)
  expect(container.resolve).not.toHaveBeenCalled()
  expect(deliver).not.toHaveBeenCalled()
})
