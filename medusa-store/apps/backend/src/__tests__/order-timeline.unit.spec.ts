import { buildOrderTimeline } from "../lib/order-timeline"
import { GET } from "../api/store/orders/[id]/timeline/route"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"

const order = { id: "o1", created_at: "2026-10-21T10:00:00Z" }

it("keeps cancellation separate from payment and never invents a refund", () => {
  const events = buildOrderTimeline({
    ...order, canceled_at: "2026-10-22T10:00:00Z",
    payment_collections: [{ payments: [{ id: "p1", amount: 959, captured_at: "2026-10-21T10:01:00Z", refunds: [] }] }],
  }, [])
  expect(events.map((event) => event.kind)).toEqual(["placed", "paid", "canceled"])
  expect(events[1].amount).toBe(959)
})

it("sorts allocation, release, partial captures, shipments and refunds by their actual time", () => {
  const events = buildOrderTimeline({
    ...order,
    payment_collections: [{ payments: [{
      id: "p1", amount: 959, captured_at: "2026-10-21T10:03:00Z",
      captures: [
        { id: "c2", amount: 459, created_at: "2026-10-21T10:03:00Z" },
        { id: "c1", amount: 500, created_at: "2026-10-21T10:01:00Z" },
      ],
      refunds: [{ id: "r1", amount: 500, created_at: "2026-10-21T10:09:00Z" }],
    }] }],
    fulfillments: [{ id: "f1", created_at: "2026-10-21T10:06:00Z", shipped_at: "2026-10-21T10:07:00Z", delivered_at: "2026-10-21T10:08:00Z" }],
  }, [
    { id: "e2", action: "release", created_at: "2026-10-21T10:04:00Z" },
    { id: "e1", action: "assign", created_at: "2026-10-21T10:02:00Z" },
    { id: "e3", action: "replace_allocation", created_at: "2026-10-21T10:05:00Z" },
    { id: "internal", action: "intake", created_at: "2026-10-21T09:00:00Z" },
  ])
  expect(events.map((event) => event.kind)).toEqual(["placed", "paid", "allocated", "paid", "released", "reallocated", "packed", "shipped", "delivered", "refund"])
  expect(events.filter((event) => event.kind === "paid").map((event) => event.amount)).toEqual([500, 459])
})

it("keeps canceled fulfillments in history without inventing shipment or delivery", () => {
  const events = buildOrderTimeline({ ...order, fulfillments: [{ id: "f1", created_at: "2026-10-21T10:01:00Z", canceled_at: "2026-10-21T10:02:00Z" }] }, [])
  expect(events.map((event) => event.kind)).toEqual(["placed", "packed", "fulfillment_canceled"])
})

it("filters by the signed-in customer and does not read device history for an inaccessible order", async () => {
  const graph = jest.fn().mockResolvedValue({ data: [] })
  const resolve = jest.fn().mockReturnValue({ graph })
  await expect(GET({ params: { id: "o1" }, auth_context: { actor_id: "other-customer" }, scope: { resolve } } as any, {} as any)).rejects.toThrow("Order not found")
  expect(graph).toHaveBeenCalledWith(expect.objectContaining({ filters: { id: "o1", customer_id: "other-customer" } }))
  expect(resolve).toHaveBeenCalledTimes(1)
  expect(resolve).toHaveBeenCalledWith(ContainerRegistrationKeys.QUERY)
})

it("returns only customer-facing event fields from device records", async () => {
  const graph = jest.fn().mockResolvedValue({ data: [{ ...order, currency_code: "eur" }] })
  const listDeviceEvents = jest.fn().mockResolvedValue([{
    id: "e1", action: "assign", created_at: "2026-10-21T10:01:00Z",
    actor_id: "internal-user", data: { cost: 100, internal_note: "private" },
  }])
  const res = { json: jest.fn() }
  await GET({ params: { id: "o1" }, auth_context: { actor_id: "c1" }, scope: { resolve: (key: string) => key === ContainerRegistrationKeys.QUERY ? { graph } : { listDeviceEvents } } } as any, res as any)
  expect(listDeviceEvents).toHaveBeenCalledWith(
    { order_id: "o1", action: ["assign", "release", "replace_allocation"] },
    expect.objectContaining({ select: ["id", "action", "created_at"] }),
  )
  expect(res.json).toHaveBeenCalledWith({ currency_code: "eur", events: [
    { id: "o1", kind: "placed", occurred_at: "2026-10-21T10:00:00.000Z" },
    { id: "e1", kind: "allocated", occurred_at: "2026-10-21T10:01:00.000Z" },
  ] })
})
