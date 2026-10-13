import { sdk } from "@lib/config"
import { placeOrder } from "@lib/data/cart"
import { getAuthHeaders, getCartId } from "@lib/data/cookies"
import { isMollie } from "@lib/constants"
import type { HttpTypes } from "@medusajs/types"
import { unstable_rethrow } from "next/navigation"
import { NextRequest, NextResponse } from "next/server"

export async function GET(req: NextRequest) {
  const cartId = req.nextUrl.searchParams.get("cart_id")
  const currentCartId = await getCartId()
  const checkout = new URL("/checkout", req.nextUrl.origin)
  checkout.searchParams.set("step", "payment")
  const retry = (status: string) => {
    checkout.searchParams.set("payment_status", status)
    return NextResponse.redirect(checkout)
  }
  // A URL alone must never replace the buyer's cart cookie or complete another cart.
  if (!cartId || cartId !== currentCartId || !/^cart_[\w]+$/.test(cartId))
    return retry("invalid")
  const cart = await sdk.client
    .fetch<HttpTypes.StoreCartResponse>(`/store/carts/${cartId}`, {
      headers: await getAuthHeaders(),
      query: {
        fields: "*payment_collection.payment_sessions,*shipping_address",
      },
      cache: "no-store",
    })
    .then(({ cart }) => cart)
    .catch(() => null)
  if (!cart) return retry("invalid")
  const country = cart.shipping_address?.country_code?.toLowerCase()
  if (country && /^[a-z]{2}$/.test(country))
    checkout.pathname = `/${country}/checkout`
  const sessions = cart.payment_collection?.payment_sessions ?? []
  if (
    !sessions.some(
      (session) =>
        isMollie(session.provider_id) &&
        ["pending", "requires_more", "authorized"].includes(session.status)
    )
  )
    return retry("failed")
  try {
    // Medusa rechecks payment with Mollie during authorization. Neither the
    // browser's query string nor a redirect is evidence that a payment succeeded.
    await placeOrder(cartId)
  } catch (error) {
    unstable_rethrow(error)
    return retry(
      error instanceof Error &&
        /canceled|cancelled|expired|failed/i.test(error.message)
        ? "failed"
        : "pending"
    )
  }
  return retry("pending")
}
