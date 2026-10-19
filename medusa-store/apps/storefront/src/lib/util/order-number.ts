export function getOrderNumber(order: {
  custom_display_id?: string | null
  display_id?: number
  id?: string
}): string {
  return (
    order.custom_display_id ||
    (order.display_id != null ? String(order.display_id) : order.id || "")
  )
}
