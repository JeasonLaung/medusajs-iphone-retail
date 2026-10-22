// A payment authorization alone does not mean the order has been paid.
export function isOrderPaidForAllocation(order: {
  payment_collections?: ({ status?: string | null } | null | undefined)[] | null
}): boolean {
  const collections = order.payment_collections
  return Boolean(collections?.length && collections.every((collection) => collection?.status === "completed"))
}
