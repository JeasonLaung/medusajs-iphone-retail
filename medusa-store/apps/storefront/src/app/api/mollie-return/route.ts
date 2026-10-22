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
  if (req.headers.get("origin") !== req.nextUrl.origin)
    return json({ status: "invalid" }, 403)
  const currentCartId = req.cookies.get("_medusa_cart_id")?.value
  const cartId = req.nextUrl.searchParams.get("cart_id") || currentCartId
  if (!currentCartId || !cartId || cartId !== currentCartId || !/^cart_[\w]+$/.test(cartId))
    return json({ status: "invalid" })
  const token = req.cookies.get("_medusa_jwt")?.value
  const headers: Record<string, string> = token ? { authorization: `Bearer ${token}` } : {}
  const locale = req.cookies.get("_medusa_locale")?.value || "en"
  const retrieve = () => sdk.client.fetch<HttpTypes.StoreCartResponse>(`/store/carts/${cartId}`, {
    headers,
    query: { fields: "*payment_collection.payment_sessions,*shipping_address" },
    cache: "no-store",
  }).then(({ cart }) => cart)
  let cart: HttpTypes.StoreCart
  try {
    cart = await retrieve()
  } catch {
    return json({ status: "error" })
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
