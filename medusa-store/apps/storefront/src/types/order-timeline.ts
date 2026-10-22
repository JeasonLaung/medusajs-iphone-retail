export type OrderTimelineKind =
  | "placed" | "paid" | "refund" | "allocated" | "released" | "reallocated"
  | "packed" | "shipped" | "delivered" | "fulfillment_canceled" | "canceled"

export type OrderTimeline = {
  currency_code: string
  events: { id: string; kind: OrderTimelineKind; occurred_at: string; amount?: number }[]
}
