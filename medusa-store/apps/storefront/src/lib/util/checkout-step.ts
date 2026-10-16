import type { HttpTypes } from "@medusajs/types"
import { getActivePaymentSession } from "./payment-session"

export type CheckoutStep = "address" | "delivery" | "payment" | "review"

export function checkoutStep(cart: HttpTypes.StoreCart, requested?: string): CheckoutStep {
  const address = cart.shipping_address
  if (!cart.email || !address?.first_name || !address.last_name || !address.address_1 || !address.city || !address.postal_code || !address.country_code || !cart.billing_address) return "address"
  if (requested === "address") return "address"
  if (!cart.shipping_methods?.length || requested === "delivery") return "delivery"
  const giftCards = (cart as unknown as { gift_cards?: unknown[] }).gift_cards
  if (requested === "review" && (getActivePaymentSession(cart) || (giftCards?.length && cart.total === 0))) return "review"
  return "payment"
}
