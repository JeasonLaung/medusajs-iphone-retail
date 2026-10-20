import { orderItemQuantities } from "./order-item-quantities"

export type AllocationFulfillment = {
  id: string
  location_id?: string | null
  shipped_at?: unknown
  delivered_at?: unknown
  canceled_at?: unknown
  items?: { line_item_id?: string | null; quantity?: unknown }[] | null
}
type Allocation = { item_id: string; status: string; fulfillment_id?: string | null }

export function orderDeviceCapacity(
  item: Parameters<typeof orderItemQuantities>[0] & { id: string },
  assignments: Allocation[],
  fulfillments: AllocationFulfillment[] = [],
) {
  const quantities = orderItemQuantities(item)
  const active = assignments.filter((a) => a.item_id === item.id && ["reserved", "packed", "shipped"].includes(a.status))
  const unfulfilled = Math.max(0, quantities.quantity - quantities.fulfilled_quantity - active.filter((a) => a.status === "reserved").length)
  const slots = fulfillments.filter((f) => !f.shipped_at && !f.delivered_at && !f.canceled_at).map((f) => ({
    ...f,
    remaining: Math.max(0, (f.items || []).filter((entry) => entry.line_item_id === item.id).reduce((total, entry) => total + orderItemQuantities({ quantity: entry.quantity }).quantity, 0) - active.filter((a) => a.fulfillment_id === f.id).length),
  })).filter((f) => f.remaining > 0)
  return { unfulfilled, slots, remaining: Math.min(Math.max(0, quantities.quantity - active.length), unfulfilled + slots.reduce((total, f) => total + f.remaining, 0)) }
}
