"use server"

import { sdk } from "@lib/config"
import { checkoutStep } from "@lib/util/checkout-step"
import { getMollieCheckoutUrl } from "@lib/util/payment-session"
import { getCheckoutCopy } from "@modules/checkout/i18n"
import { initiatePaymentSession } from "./cart"
import { getAuthHeaders, getCartId } from "./cookies"
import { getLocale } from "./locale-actions"
import { listCartPaymentMethods } from "./payment"

export async function startMolliePayment() {
  const copy = getCheckoutCopy((await getLocale()) || "en")
  const cartId = await getCartId()
  if (!cartId) throw new Error(copy.paymentError)
  // Read the saved delivery method and current total before creating payment.
  const { cart } = await sdk.store.cart.retrieve(cartId, {
    fields: "*shipping_address,*billing_address,*shipping_methods,*payment_collection.payment_sessions",
  }, await getAuthHeaders())
  if (checkoutStep(cart, "payment") !== "payment")
    throw new Error(copy.completeDelivery)
  const providerId = "pp_mollie-hosted-checkout_mollie"
  const methods = await listCartPaymentMethods(cart.region_id!)
  if (!methods?.some((method) => method.id === providerId))
    throw new Error(copy.paymentUnavailable)
  const response = await initiatePaymentSession(cart, { provider_id: providerId }).catch((error: unknown) => {
    if (error instanceof Error && /MOLLIE_MULTIPLE_PAID_SESSIONS|MOLLIE_PAID_AMOUNT_MISMATCH/i.test(error.message))
      throw new Error(copy.paymentNeedsReview)
    throw error
  })
  const session = response.payment_collection.payment_sessions?.find(
    (entry) => entry.provider_id === providerId &&
      (entry.status === "pending" || entry.status === "requires_more" || entry.status === "authorized")
  )
  if (session?.status === "authorized")
    return `/api/mollie-return?cart_id=${encodeURIComponent(cartId)}`
  const url = getMollieCheckoutUrl(session?.data)
  if (!url) throw new Error(copy.paymentError)
  return url
}
