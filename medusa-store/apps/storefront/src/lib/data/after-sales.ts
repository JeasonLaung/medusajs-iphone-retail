"use server"

import { sdk } from "@lib/config"
import { getAuthHeaders } from "./cookies"

export type OrderDevice = { id: string; assignment_id: string; item_id: string; serial_number: string | null; imei: string | null; status: string; replacement_of: string | null; tracking_number: string | null }
export async function listOrderDevices(orderId: string): Promise<OrderDevice[]> {
  const headers = await getAuthHeaders()
  if (!headers.authorization) return []
  const { devices } = await sdk.client.fetch<{ devices: OrderDevice[] }>(`/store/orders/${orderId}/devices`, { headers, cache: "no-store" })
  return devices
}

export type AfterSalesRequest = {
  id: string
  type: "return" | "exchange" | "repair"
  status: "pending" | "processing" | "completed" | "rejected"
  reason: string
  items: { item_id: string; quantity: number; title: string; device_ids?: string[] }[]
  created_at: string
}

export async function listAfterSales(orderId: string) {
  const headers = await getAuthHeaders()
  if (!headers.authorization) return { requests: [], loginRequired: true, error: null }
  try {
    const { requests } = await sdk.client.fetch<{ requests: AfterSalesRequest[] }>(`/store/orders/${orderId}/after-sales`, {
      headers, cache: "no-store",
    })
    return { requests, loginRequired: false, error: null }
  } catch (error) {
    const status = (error as { status?: number }).status
    return { requests: [], loginRequired: status === 401, error: status === 404
      ? "This order is not linked to your account. Add it in My orders before requesting support."
      : status === 401 ? null : "Unable to load requests. Please try again." }
  }
}

export async function submitAfterSales(orderId: string, body: {
  type: AfterSalesRequest["type"]
  reason: string
  items: { item_id: string; quantity: number; device_ids?: string[] }[]
}) {
  const headers = await getAuthHeaders()
  if (!headers.authorization) return { request: null, error: "Sign in to request after-sales support." }
  try {
    const { request } = await sdk.client.fetch<{ request: AfterSalesRequest }>(`/store/orders/${orderId}/after-sales`, {
      method: "POST", headers, body, cache: "no-store",
    })
    return { request, error: null }
  } catch (error) {
    const status = (error as { status?: number }).status
    return { request: null, error: status === 401 ? "Your session expired. Please sign in again."
      : status === 404 ? "This order is not linked to your account."
      : status === 400 || status === 409 ? (error as Error).message
      : "Unable to submit your request. Please try again." }
  }
}
