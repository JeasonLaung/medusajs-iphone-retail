"use server"

import { sdk } from "@lib/config"
import { getAuthHeaders } from "./cookies"
import type {
  ProductReviewPage,
  ReviewPurchases,
  OrderReviewContext,
} from "types/product-reviews"

export async function listProductReviews(
  productId: string,
  offset = 0
): Promise<ProductReviewPage | null> {
  try {
    return await sdk.client.fetch<ProductReviewPage>("/store/product-reviews", {
      query: { product_id: productId, offset, limit: 5 },
      cache: "no-store",
    })
  } catch {
    return null
  }
}

export async function submitProductReview(input: {
  product_id: string
  order_id?: string
  rating: number
  title: string
  content: string
}) {
  const headers = await getAuthHeaders()
  if (!headers || !("authorization" in headers))
    return { error: "login" as const }
  try {
    await sdk.client.fetch("/store/product-reviews", {
      method: "POST",
      body: input,
      headers,
    })
    return { error: null }
  } catch (error) {
    const failure = error as { status?: number; message?: string }
    if (failure.status === 401) return { error: "login" as const }
    if (
      failure.status === 409 ||
      failure.message?.includes("review.already_submitted")
    )
      return { error: "duplicate" as const }
    if (failure.message?.includes("review.order_"))
      return { error: "order" as const }
    return { error: "submit" as const }
  }
}

export async function getReviewPurchases(
  productId: string,
  offset = 0
): Promise<ReviewPurchases | null> {
  try {
    return await sdk.client.fetch<ReviewPurchases>(
      "/store/product-reviews/purchases",
      {
        query: { product_id: productId, offset },
        headers: await getAuthHeaders(),
        cache: "no-store",
      }
    )
  } catch {
    return null
  }
}

export async function getOrderReviewContext(
  orderId: string
): Promise<OrderReviewContext | null> {
  try {
    return await sdk.client.fetch<OrderReviewContext>(
      `/store/orders/${orderId}/product-reviews`,
      {
        headers: await getAuthHeaders(),
        cache: "no-store",
      }
    )
  } catch {
    return null
  }
}
