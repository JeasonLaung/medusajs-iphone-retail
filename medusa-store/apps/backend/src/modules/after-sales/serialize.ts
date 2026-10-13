export function serializeAfterSales<T extends { items: Record<string, unknown> }>(request: T) {
  return { ...request, items: request.items.lines }
}
