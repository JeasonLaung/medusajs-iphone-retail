export function getOrderNumber(order: {
  custom_display_id?: string | null
  display_id: number
}): string {
  return order.custom_display_id || String(order.display_id)
}
