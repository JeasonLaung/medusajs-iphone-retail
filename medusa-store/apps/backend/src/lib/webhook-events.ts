// Shared by the API validator, subscriber, and Admin subscription form.
export const webhookEvents = [
  "order.placed", "order.canceled", "order.completed",
  "order.fulfillment_created", "order.fulfillment_canceled",
  "payment.captured", "payment.refunded",
  "shipment.created", "delivery.created",
  "after_sales.created", "after_sales.processing", "after_sales.completed", "after_sales.rejected",
  "order.return_requested", "order.return_received", "order.exchange_created", "order.claim_created",
] as const

export type WebhookEvent = typeof webhookEvents[number]

export const webhookEventLabels: Record<WebhookEvent, readonly [string, string]> = {
  "order.placed": ["新订单", "Order placed"],
  "order.canceled": ["订单取消", "Order canceled"],
  "order.completed": ["订单完成", "Order completed"],
  "order.fulfillment_created": ["订单已创建履约", "Order fulfillment created"],
  "order.fulfillment_canceled": ["订单履约已取消", "Order fulfillment canceled"],
  "payment.captured": ["收款成功", "Payment captured"],
  "payment.refunded": ["退款成功", "Payment refunded"],
  "shipment.created": ["已创建发货", "Shipment created"],
  "delivery.created": ["已确认送达", "Delivery confirmed"],
  "after_sales.created": ["新售后申请", "After-sales request submitted"],
  "after_sales.processing": ["售后处理中", "After-sales request processing"],
  "after_sales.completed": ["售后已完成", "After-sales request completed"],
  "after_sales.rejected": ["售后已拒绝", "After-sales request rejected"],
  "order.return_requested": ["退货已确认", "Return confirmed"],
  "order.return_received": ["退货已收货", "Return received"],
  "order.exchange_created": ["换货已确认", "Exchange confirmed"],
  "order.claim_created": ["索赔已确认", "Claim confirmed"],
}
