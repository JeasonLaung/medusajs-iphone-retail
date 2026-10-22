import { createStep, createWorkflow } from "@medusajs/framework/workflows-sdk"
import { cancelOrderWorkflow } from "@medusajs/medusa/core-flows"
import { CancelOrderBody } from "../api/admin/orders/[id]/cancel/validators"
import { transformOrderCancellation } from "../lib/admin-order-cancellation"
import "../workflows/cancel-order-with-reason"

jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn(() => jest.fn(input => ({ order_id: input.order_id, canceled_by: input.actor_id }))),
  createWorkflow: jest.fn(() => ({})),
  StepResponse: class { constructor(public output: unknown, public compensation: unknown) {} },
  WorkflowResponse: jest.fn(),
}))
jest.mock("@medusajs/medusa/core-flows", () => ({ cancelOrderWorkflow: { runAsStep: jest.fn() } }))

const [, save, compensate] = (createStep as jest.Mock).mock.calls.find(([name]) => name === "record-order-cancellation-reason")
const input = { order_id: "o1", actor_id: "u1", reason: "  Customer requested cancellation\nDuplicate order  " }
function setup(extra = {}) {
  const order = { id: "o1", status: "pending", canceled_at: null, metadata: { keep: "original" }, ...extra }
  const orders = { retrieveOrder: jest.fn().mockResolvedValue(order), updateOrders: jest.fn() }
  return { order, orders, container: { resolve: () => orders } }
}

test.each([{}, { reason: "" }, { reason: " \n " }, { reason: "a".repeat(1001) }])("rejects missing, blank and overlong cancellation reasons: %j", body => {
  expect(CancelOrderBody.safeParse(body).success).toBe(false)
})
test("validates before writing and stores reason, actor and time without replacing other metadata", async () => {
  const { orders, container } = setup()
  await expect(save({ ...input, reason: " " }, { container })).rejects.toThrow()
  expect(orders.updateOrders).not.toHaveBeenCalled()
  const result = await save(input, { container })
  expect(orders.updateOrders).toHaveBeenCalledWith([{
    id: "o1", metadata: {
      keep: "original", cancellation: { reason: input.reason.trim(), actor_id: "u1", recorded_at: expect.any(String) },
    },
  }])
  expect(result.output).toEqual({ order_id: "o1", canceled_by: "u1" })
  expect(result.compensation).toEqual({ order_id: "o1", previous: null })
})
test("a repeated cancellation cannot overwrite the recorded reason", async () => {
  const { orders, container } = setup({ status: "canceled", canceled_at: new Date() })
  await expect(save(input, { container })).rejects.toThrow("already been canceled")
  expect(orders.updateOrders).not.toHaveBeenCalled()
})
test("restores the previous audit value when native cancellation fails and preserves unrelated concurrent metadata", async () => {
  const { orders, container } = setup({ metadata: { keep: "changed", cancellation: { reason: "new" } } })
  const previous = { reason: "old" }
  await compensate({ order_id: "o1", previous }, { container })
  expect(orders.updateOrders).toHaveBeenCalledWith([{ id: "o1", metadata: { keep: "changed", cancellation: previous } }])
})
test("keeps the reason if cancellation already completed before a later failure", async () => {
  const { orders, container } = setup({ canceled_at: new Date() })
  await compensate({ order_id: "o1", previous: null }, { container })
  expect(orders.updateOrders).not.toHaveBeenCalled()
})
test("runs the native cancel workflow using the preceding audit step output", () => {
  const compose = (createWorkflow as jest.Mock).mock.calls.find(([name]) => name === "cancel-order-with-reason")[1]
  compose(input)
  expect(cancelOrderWorkflow.runAsStep).toHaveBeenCalledWith({ input: { order_id: "o1", canceled_by: "u1" } })
})
test("dashboard upgrades cannot silently lose cancellation reason collection", () => {
  expect(() => transformOrderCancellation("changed")).toThrow("dashboard action changed")
})
