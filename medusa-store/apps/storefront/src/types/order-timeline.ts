export type OrderTimelineKind =
  | "placed" | "paid" | "refund" | "allocated" | "released" | "reallocated"
  | "packed" | "shipped" | "delivered" | "fulfillment_canceled" | "canceled"

export type OrderShipment = {
  official_demo?: boolean
  id: string
  shipped_at: string | null
  delivered_at: string | null
  labels: { tracking_number: string; tracking_url: string }[]
}

export type OrderTimeline = {
  shipments?: OrderShipment[]
  currency_code: string
  events: { id: string; kind: OrderTimelineKind; occurred_at: string; amount?: number }[]
}
