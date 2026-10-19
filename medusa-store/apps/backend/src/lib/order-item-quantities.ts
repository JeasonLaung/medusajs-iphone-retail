function quantity(value: unknown): number | undefined {
  if (value == null || value === "" || typeof value === "boolean")
    return undefined
  if (typeof value === "object")
    return quantity((value as { value?: unknown }).value)
  const number = Number(value)
  return Number.isFinite(number) && number >= 0 ? number : undefined
}

export function orderItemQuantities(item: {
  quantity?: unknown
  raw_quantity?: unknown
  detail?: {
    quantity?: unknown
    raw_quantity?: unknown
    fulfilled_quantity?: unknown
    raw_fulfilled_quantity?: unknown
  } | null
}) {
  return {
    quantity:
      quantity(item.quantity) ??
      quantity(item.detail?.quantity) ??
      quantity(item.raw_quantity) ??
      quantity(item.detail?.raw_quantity) ??
      0,
    fulfilled_quantity:
      quantity(item.detail?.fulfilled_quantity) ??
      quantity(item.detail?.raw_fulfilled_quantity) ??
      0,
  }
}
