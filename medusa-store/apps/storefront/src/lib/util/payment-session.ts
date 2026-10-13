import type { HttpTypes } from "@medusajs/types"

export const getActivePaymentSession = (cart: HttpTypes.StoreCart) =>
  cart.payment_collection?.payment_sessions?.find(
    (session) => session.status === "authorized"
  ) ??
  cart.payment_collection?.payment_sessions?.find(
    (session) =>
      session.status === "pending" || session.status === "requires_more"
  )

export const getMollieCheckoutUrl = (data?: Record<string, unknown>) => {
  const links = data?._links as { checkout?: { href?: unknown } } | undefined
  const href = links?.checkout?.href
  if (typeof href !== "string") return null
  try {
    const url = new URL(href)
    if (
      url.protocol !== "https:" ||
      url.username ||
      url.password ||
      !(url.hostname === "mollie.com" || url.hostname.endsWith(".mollie.com"))
    )
      return null
    return url.href
  } catch {
    return null
  }
}
