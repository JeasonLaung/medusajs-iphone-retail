import { createStep } from "@medusajs/framework/workflows-sdk"
import subscriber, { config } from "../subscribers/customer-notifications"
import { createCustomerNoticeWorkflow } from "../workflows/customer-notifications"
import { loadWebhookContext } from "../modules/webhook-notifications/message"
import "../workflows/buyback"

jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn(() => ({})), createWorkflow: jest.fn(() => ({})), transform: jest.fn(),
  StepResponse: class { constructor(public output: unknown) {} }, WorkflowResponse: jest.fn(),
}))
jest.mock("@medusajs/medusa/core-flows", () => ({}))
jest.mock("../workflows/customer-notifications", () => ({ createCustomerNoticeWorkflow: jest.fn(), createCustomerNoticeStep: jest.fn() }))
jest.mock("../modules/webhook-notifications/message", () => ({ loadWebhookContext: jest.fn() }))

const prepareBuyback = (createStep as jest.Mock).mock.calls.find(([name]) => name === "buyback-notice-input")[1]

beforeEach(() => jest.clearAllMocks())

it.each(config.event as string[])("creates an owned notification for %s", async name => {
  const run = jest.fn()
  jest.mocked(createCustomerNoticeWorkflow).mockReturnValue({ run } as any)
  jest.mocked(loadWebhookContext).mockResolvedValue({ resourceId: "resource1", order: { id: "order1", display_id: 123, currency_code: "eur" } })
  const graph = jest.fn().mockResolvedValue({ data: [{ id: "order1", customer_id: "customer1" }] })
  const container = { resolve: () => ({ graph }) }
  await subscriber({ event: { name, data: { id: "resource1" } }, container } as any)
  expect(run).toHaveBeenCalledWith({ input: expect.objectContaining({ customer_id: "customer1", kind: name, resource_id: "order1", event_key: `${name}:resource1`, reference: "123" }) })
})

it("does not assign a guest order's notifications to another customer", async () => {
  jest.mocked(loadWebhookContext).mockResolvedValue({ resourceId: "resource1", order: { id: "order1", currency_code: "eur" } })
  await subscriber({ event: { name: "shipment.created", data: { id: "resource1" } }, container: { resolve: () => ({ graph: async () => ({ data: [{ id: "order1", customer_id: null }] }) }) } } as any)
  expect(createCustomerNoticeWorkflow).not.toHaveBeenCalled()
})

it.each(["review_photo", "approve_shipping", "mark_received", "final_offer", "mark_paid", "request_return", "mark_returning", "mark_returned", "cancel"])("notifies the request owner after Admin action %s", async action => {
  const request = { id: "buyback1", customer_id: "customer1", history: [{ at: "2026-10-23T08:00:00Z" }] }
  const container = { resolve: () => ({ retrieveBuybackRequest: async () => request }) }
  const response = await prepareBuyback({ request_id: request.id, actor_id: "admin1", command: { action } }, { container })
  expect(response.output).toEqual({ customer_id: "customer1", event_key: "buyback:buyback1:2026-10-23T08:00:00Z", kind: "buyback.updated", resource_id: request.id, reference: request.id, action })
})

it.each([{ customer_id: "customer1", command: { action: "accept_offer" } }, { actor_id: "admin1", command: { action: "link_device" } }])("skips customer actions and internal device links", async input => {
  const container = { resolve: jest.fn() }
  expect((await prepareBuyback(input, { container })).output).toBeNull()
  expect(container.resolve).not.toHaveBeenCalled()
})
