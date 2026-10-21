import type { HttpTypes } from "@medusajs/types"
import type { AccountKey } from "@modules/account/i18n"

export const orderStatusKeys: Record<string, AccountKey> = {
  pending: "orderPending",
  completed: "completed",
  draft: "orderDraft",
  archived: "orderArchived",
  canceled: "fulfillmentCanceled",
  requires_action: "paymentRequiresAction",
}

export const paymentStatusKeys: Record<string, AccountKey> = {
  not_paid: "paymentNotPaid",
  awaiting: "paymentAwaiting",
  authorized: "paymentAuthorized",
  partially_authorized: "paymentPartiallyAuthorized",
  captured: "paymentCaptured",
  partially_captured: "paymentPartiallyCaptured",
  partially_refunded: "paymentPartiallyRefunded",
  refunded: "paymentRefunded",
  canceled: "paymentCanceled",
  requires_action: "paymentRequiresAction",
}

export function getOrderStatusKey(
  order: Pick<
    HttpTypes.StoreOrder,
    "status" | "payment_status" | "fulfillment_status"
  >
): AccountKey {
  if (order.status === "canceled") return "fulfillmentCanceled"
  if (order.payment_status !== "captured") {
    return paymentStatusKeys[order.payment_status] || "statusUnknown"
  }
  if (order.status === "completed" || order.status === "archived") {
    return orderStatusKeys[order.status]
  }
  const fulfillmentKeys: Record<string, AccountKey> = {
    not_fulfilled: "paidAwaitingFulfillment",
    partially_fulfilled: "fulfillmentPartiallyFulfilled",
    fulfilled: "fulfillmentFulfilled",
    partially_shipped: "fulfillmentPartiallyShipped",
    shipped: "fulfillmentShipped",
    partially_delivered: "fulfillmentPartiallyDelivered",
    delivered: "fulfillmentDelivered",
    canceled: "fulfillmentCanceled",
  }
  return fulfillmentKeys[order.fulfillment_status] || "paymentCaptured"
}

// Element's light tag fills and primary text keep small labels readable.
export function getOrderStatusClasses(key: AccountKey): string {
  switch (key) {
    case "completed":
    case "fulfillmentDelivered":
      return "bg-[#F0F9EB] border-[#67C23A] text-[#303133]"
    case "paymentRequiresAction":
      return "bg-[#FEF0F0] border-[#F56C6C] text-[#303133]"
    case "paidAwaitingFulfillment":
    case "paymentCaptured":
    case "fulfillmentPartiallyFulfilled":
    case "fulfillmentFulfilled":
    case "fulfillmentPartiallyShipped":
    case "fulfillmentShipped":
    case "fulfillmentPartiallyDelivered":
      return "bg-[#ECF5FF] border-[#409EFF] text-[#303133]"
    case "paymentNotPaid":
    case "paymentAwaiting":
    case "paymentAuthorized":
    case "paymentPartiallyAuthorized":
    case "paymentPartiallyCaptured":
    case "orderPending":
      return "bg-[#FDF6EC] border-[#E6A23C] text-[#303133]"
    default:
      return "bg-[#F4F4F5] border-[#909399] text-[#303133]"
  }
}

export function getOrderOriginalTotal(
  order: Pick<
    HttpTypes.StoreOrder,
    "total" | "original_total" | "discount_total"
  >
): number | null {
  // Credit lines and refunds alone are not a sale price.
  if (!(order.discount_total > 0)) return null
  return Number.isFinite(order.original_total) &&
    order.original_total > order.total
    ? order.original_total
    : null
}

// For split payments, show the most recent actual capture. Payment creation
// and authorization timestamps do not mean that funds have been collected.
export function getOrderPaidAt(
  order: Pick<HttpTypes.StoreOrder, "payment_collections">
): string | null {
  const capturedTimes = (order.payment_collections ?? []).flatMap(
    (collection) =>
      (collection.payments ?? []).flatMap((payment) => {
        if (!payment.captured_at) return []
        const timestamp = new Date(payment.captured_at).getTime()
        return Number.isFinite(timestamp) ? [timestamp] : []
      })
  )
  return capturedTimes.length
    ? new Date(Math.max(...capturedTimes)).toISOString()
    : null
}
