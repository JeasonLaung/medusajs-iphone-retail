import { sdk } from "@lib/config"
import { isMollie } from "@lib/constants"
import { storefrontSegment } from "@lib/util/storefront-route"
import type { HttpTypes } from "@medusajs/types"
import { revalidateTag } from "next/cache"
import { NextRequest, NextResponse } from "next/server"

// Keep this URL stable for payments that have already been created at Mollie.
// The browser lands on a dedicated page before payment verification starts.
export async function GET(req: NextRequest) {
  const cartId = req.nextUrl.searchParams.get("cart_id") || req.cookies.get("_medusa_cart_id")?.value
  const locale = req.cookies.get("_medusa_locale")?.value || "en"
  const country = req.nextUrl.searchParams.get("country_code")
  const path = country && /^[a-z]{2}$/.test(country)
    ? `/${storefrontSegment(country, locale)}/payment-result`
    : "/payment-result"
  const url = new URL(path, req.nextUrl.origin)
  if (cartId) url.searchParams.set("cart_id", cartId)
  return NextResponse.redirect(url)
}

export async function POST(req: NextRequest) {
  const json = (data: Record<string, unknown>, status = 200) =>
    NextResponse.json(data, { status, headers: { "Cache-Control": "no-store" } })
  // HTTPS proxies can expose an internal HTTP origin to Next.js. Trust only
  // the explicit storefront configuration in addition to the request origin.
  const storefrontOrigin = process.env.NEXT_PUBLIC_BASE_URL
    ? new URL(process.env.NEXT_PUBLIC_BASE_URL).origin
    : req.nextUrl.origin
  const origin = req.headers.get("origin")
  if (origin !== req.nextUrl.origin && origin !== storefrontOrigin)
    return json({ status: "invalid" }, 403)
  const currentCartId = req.cookies.get("_medusa_cart_id")?.value
  const cartId = req.nextUrl.searchParams.get("cart_id") || currentCartId
  if (!cartId || !/^cart_[\w]+$/.test(cartId))
    return json({ status: "invalid" })
  const token = req.cookies.get("_medusa_jwt")?.value
  const remembered = req.cookies.get(`_medusa_mollie_${cartId}`)?.value === cartId
  const browserOwnsCart = cartId === currentCartId || remembered
  if (!browserOwnsCart && !token) {
    console.warn("[Mollie return] No browser payment reference", { cart_id: cartId, has_current_cart: !!currentCartId })
    return json({ status: "invalid" })
  }
  const headers: Record<string, string> = token ? { authorization: `Bearer ${token}` } : {}
  const locale = req.cookies.get("_medusa_locale")?.value || "en"
  const retrieve = () => sdk.client.fetch<HttpTypes.StoreCartResponse>(`/store/carts/${cartId}`, {
    headers,
    query: { fields: "id,customer_id,completed_at,*payment_collection.payment_sessions,*shipping_address" },
    cache: "no-store",
  }).then(({ cart }) => cart)
  let cart: HttpTypes.StoreCart
  try {
    cart = await retrieve()
  } catch {
    return json({ status: "error" })
  }
  if (!browserOwnsCart) {
    // Older payments have no dedicated return cookie. Recover them only for
    // the authenticated owner, never from the cart ID in a URL alone.
    try {
      const { customer } = await sdk.store.customer.retrieve({ fields: "id" }, headers)
      if (!cart.customer_id || customer.id !== cart.customer_id) {
        console.warn("[Mollie return] Customer does not own payment cart", { cart_id: cartId })
        return json({ status: "invalid" })
      }
    } catch (error) {
      const status = (error as { status?: number })?.status
      return json({ status: status === 401 || status === 403 ? "invalid" : "error" })
    }
  }
  const country = cart.shipping_address?.country_code?.toLowerCase()
  const segment = storefrontSegment(country && /^[a-z]{2}$/.test(country) ? country : "dk", locale)
  const mollieSessions = (value: HttpTypes.StoreCart) =>
    (value.payment_collection?.payment_sessions ?? []).filter((session) => isMollie(session.provider_id))
  const sessions = mollieSessions(cart)
  if (!sessions.length) return json({ status: "invalid" })
  if (!sessions.some((session) => ["pending", "requires_more", "authorized", "captured"].includes(session.status)))
    return json({ status: "failed", retry_url: `/${segment}/checkout?step=delivery` })
  let result: HttpTypes.StoreCompleteCartResponse | undefined
  try {
    // Only Medusa's provider authorization can confirm payment. A browser
    // redirect, query parameter or webhook timing cannot establish success.
    result = await sdk.store.cart.complete(cartId, {}, headers)
  } catch {
    // Authorization can persist a canceled/error status before rejecting.
    // Re-read it instead of classifying SDK error messages as payment evidence.
  }
  if (result?.type === "order") {
    const cacheId = req.cookies.get("_medusa_cache_id")?.value
    if (cacheId) {
      try {
        revalidateTag(`carts-${cacheId}`)
        revalidateTag(`orders-${cacheId}`)
      } catch {
        // Cache maintenance must not hide a successfully completed order.
        console.error("[Mollie return] Unable to invalidate checkout caches after order confirmation")
      }
    }
    const response = json({ status: "confirmed", order_url: `/${segment}/order/${result.order.id}/confirmed` })
    response.cookies.set(`_medusa_mollie_${cartId}`, cartId, {
      maxAge: 60 * 60 * 24 * 7, path: "/", httpOnly: true,
      sameSite: "lax", secure: process.env.NODE_ENV === "production",
    })
    if (currentCartId === cartId)
      response.cookies.set("_medusa_cart_id", "", { maxAge: -1, path: "/" })
    return response
  }
  try {
    const latest = mollieSessions(await retrieve())
    if (latest.length && latest.every((session) => ["canceled", "error"].includes(session.status)))
      return json({ status: "failed", retry_url: `/${segment}/checkout?step=delivery` })
    return json({ status: "pending" })
  } catch {
    return json({ status: "error" })
  }
}
