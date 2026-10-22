type Timestamp = string | Date | null | undefined

export type OrderTimelineEvent = {
  id: string
  kind: "placed" | "paid" | "refund" | "allocated" | "released" | "reallocated" | "packed" | "shipped" | "delivered" | "fulfillment_canceled" | "canceled"
  occurred_at: string
  amount?: number
}

type TimelineOrder = {
  id: string
  created_at: Timestamp
  canceled_at?: Timestamp
  payment_collections?: ({ payments?: ({
    id: string
    amount: unknown
    captured_at?: Timestamp
    captures?: ({ id: string; amount: unknown; created_at: Timestamp } | null)[] | null
    refunds?: ({ id: string; amount: unknown; created_at: Timestamp } | null)[] | null
  } | null)[] | null } | null)[] | null
  fulfillments?: ({ id: string; created_at: Timestamp; shipped_at?: Timestamp; delivered_at?: Timestamp; canceled_at?: Timestamp } | null)[] | null
}

export function buildOrderTimeline(
  order: TimelineOrder,
  deviceEvents: { id: string; action: string; created_at: Timestamp }[],
): OrderTimelineEvent[] {
  const events: OrderTimelineEvent[] = []
  const add = (id: string, kind: OrderTimelineEvent["kind"], timestamp: Timestamp, amount?: unknown) => {
    if (!timestamp) return
    const date = new Date(timestamp)
    if (!Number.isFinite(date.getTime())) return
    const value = amount == null ? undefined : Number(amount)
    events.push({ id, kind, occurred_at: date.toISOString(), ...(value !== undefined && Number.isFinite(value) ? { amount: value } : {}) })
  }
  add(order.id, "placed", order.created_at)
  for (const collection of order.payment_collections || []) {
    for (const payment of collection?.payments || []) {
      if (!payment) continue
      if (payment.captures?.length) {
        for (const capture of payment.captures) {
          if (capture) add(capture.id, "paid", capture.created_at, capture.amount)
        }
      } else {
        add(payment.id, "paid", payment.captured_at, payment.amount)
      }
      for (const refund of payment.refunds || []) {
        if (refund) add(refund.id, "refund", refund.created_at, refund.amount)
      }
    }
  }
  const allocationKinds: Record<string, OrderTimelineEvent["kind"]> = {
    assign: "allocated", release: "released", replace_allocation: "reallocated",
  }
  for (const event of deviceEvents) {
    if (allocationKinds[event.action]) add(event.id, allocationKinds[event.action], event.created_at)
  }
  for (const fulfillment of order.fulfillments || []) {
    if (!fulfillment) continue
    add(`${fulfillment.id}:packed`, "packed", fulfillment.created_at)
    add(`${fulfillment.id}:shipped`, "shipped", fulfillment.shipped_at)
    add(`${fulfillment.id}:delivered`, "delivered", fulfillment.delivered_at)
    add(`${fulfillment.id}:canceled`, "fulfillment_canceled", fulfillment.canceled_at)
  }
  add(`${order.id}:canceled`, "canceled", order.canceled_at)
  return events.sort((a, b) => a.occurred_at.localeCompare(b.occurred_at))
}
