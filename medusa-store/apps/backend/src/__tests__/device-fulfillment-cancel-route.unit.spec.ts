import { POST } from "../api/admin/orders/[id]/fulfillments/[fulfillment_id]/cancel/route"
import { deviceOperationWorkflow } from "../workflows/device-operations"
import { guardDeviceOrder } from "../api/device-guards"

jest.mock("../workflows/device-operations", () => ({ deviceOperationWorkflow: jest.fn() }))

it("passes native cancellation through the guard to the synchronizing route", async () => {
  const next = jest.fn()
  const resolve = jest.fn()
  await guardDeviceOrder({ path: "/admin/orders/o1/fulfillments/f1/cancel", params: { id: "o1", fulfillment_id: "f1" }, scope: { resolve } } as any, {} as any, next)
  expect(next).toHaveBeenCalledTimes(1)
  expect(resolve).not.toHaveBeenCalled()
})

it("forwards the order identity, operator and notification preference, returning the native response", async () => {
  const run = jest.fn().mockResolvedValue({})
  ;(deviceOperationWorkflow as unknown as jest.Mock).mockReturnValue({ run })
  const graph = jest.fn().mockResolvedValue({ data: [{ id: "o1", fulfillments: [] }] })
  const res = { status: jest.fn().mockReturnThis(), json: jest.fn() }
  await POST({ params: { id: "o1", fulfillment_id: "f1" }, auth_context: { actor_id: "u1" }, validatedBody: { no_notification: true }, queryConfig: { fields: ["id", "fulfillments.*"] }, scope: { resolve: () => ({ graph }) } } as any, res as any)
  expect(run).toHaveBeenCalledWith({ input: { action: "cancel_fulfillment", fulfillment_id: "f1", actor_id: "u1", native_cancel: { order_id: "o1", canceled_by: "u1", no_notification: true } } })
  expect(res.status).toHaveBeenCalledWith(200)
  expect(res.json).toHaveBeenCalledWith({ order: { id: "o1", fulfillments: [] } })
})
