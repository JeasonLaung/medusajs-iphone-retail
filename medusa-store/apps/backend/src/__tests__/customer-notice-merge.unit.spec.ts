import { createStep } from "@medusajs/framework/workflows-sdk"
import "../workflows/customer-notifications"

jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn(() => ({})), createWorkflow: jest.fn(() => ({})),
  StepResponse: class { constructor(public output: unknown, public compensationInput: unknown) {} }, WorkflowResponse: jest.fn(),
}))
jest.mock("@medusajs/medusa/core-flows", () => ({}))

const [, create, compensate] = (createStep as jest.Mock).mock.calls.find(([name]) => name === "create-customer-notice")
const input = { customer_id: "customer1", event_key: "buyback:request1:event2", kind: "buyback.updated", resource_id: "request1", reference: "request1", action: "final_offer" }
const unread = { ...input, id: "notice1", event_key: "buyback:request1:event1", action: "mark_received" }
let service: any
let container: any
beforeEach(() => {
  service = {
    listCustomerNotices: jest.fn().mockResolvedValueOnce([]).mockResolvedValue([unread]),
    updateCustomerNotices: jest.fn().mockResolvedValue([{ ...input, id: unread.id }]),
    createCustomerNotices: jest.fn().mockResolvedValue({ ...input, id: "notice2" }),
    deleteCustomerNotices: jest.fn(),
  }
  container = { resolve: () => service }
})

it("merges the latest action into the same owner's unread request notice", async () => {
  const response = await create(input, { container })
  expect(service.listCustomerNotices).toHaveBeenLastCalledWith({ customer_id: "customer1", kind: "buyback.updated", resource_id: "request1", read_at: null }, expect.anything())
  expect(response.output).toEqual({ ...input, id: "notice1" })
  expect(service.createCustomerNotices).not.toHaveBeenCalled()
  await compensate(response.compensationInput, { container })
  expect(service.updateCustomerNotices).toHaveBeenLastCalledWith({ id: unread.id, event_key: unread.event_key, action: unread.action, reference: unread.reference })
  expect(service.deleteCustomerNotices).not.toHaveBeenCalled()
})

it("creates a new reminder after the previous one was read", async () => {
  service.listCustomerNotices.mockReset().mockResolvedValue([])
  const response = await create(input, { container })
  expect(service.createCustomerNotices).toHaveBeenCalledWith(input)
  await compensate(response.compensationInput, { container })
  expect(service.deleteCustomerNotices).toHaveBeenCalledWith("notice2")
})

it("creates a reminder if the existing notice is read during merging", async () => {
  service.updateCustomerNotices.mockResolvedValue([])
  await create(input, { container })
  expect(service.createCustomerNotices).toHaveBeenCalledWith(input)
})

it("does not merge order notifications", async () => {
  await create({ ...input, kind: "shipment.created" }, { container })
  expect(service.listCustomerNotices).toHaveBeenCalledTimes(1)
  expect(service.createCustomerNotices).toHaveBeenCalled()
})

it("does not change the same event on retry", async () => {
  service.listCustomerNotices.mockReset().mockResolvedValue([unread])
  const response = await create(input, { container })
  expect(response.compensationInput).toBeNull()
  expect(service.updateCustomerNotices).not.toHaveBeenCalled()
  expect(service.createCustomerNotices).not.toHaveBeenCalled()
})
