"use server"

import { sdk } from "@lib/config"
import { getAuthHeaders } from "./cookies"

export type BuybackAssessment = { battery: string; checks: (boolean | null)[]; faults: string[]; glass: string[]; screen: string; frame: string; back: string }
export type BuybackItem = { id: string; product_id: string; model: string; model_name: string; capacity: string; assessment: BuybackAssessment; reference_amount: number | null; final_amount: number | null; inspection_notes: string; serial_number: string; imei: string; device_id: string | null }
export type BuybackPhoto = { id: string; item_id: string; photo_type: string; uploaded_at: string; review_status: string; review_note: string; mime_type: string }
export type BuybackAddress = { name: string; address_1: string; address_2: string; city: string; postal_code: string; country_code: string }
export type BuybackRequest = {
  id: string; status: string; contact_name: string; email: string; phone: string; return_address: BuybackAddress;
  items: BuybackItem[]; photos: BuybackPhoto[]; reference_amount: number | null; final_amount: number | null; currency_code: string;
  shipping_instructions: string; tracking_number: string; return_tracking: string; payment_reference: string; review_note: string;
  created_at: string; retention_days: number; history: { at: string; action: string; actor?: "customer" | "platform"; status: string; note: string }[]
}
export type BuybackDraft = { submission_key: string; contact_name: string; phone: string; return_address: BuybackAddress; reference_acknowledged: true; privacy_acknowledged: true; items: { model: string; capacity: string; assessment: BuybackAssessment }[] }
export type BuybackCommand = { action: "submit_photos"; items: { item_id: string; serial_number: string; imei: string }[] } | { action: "mark_shipped"; tracking_number: string } | { action: "accept_offer"; final_amount: number } | { action: "decline_offer" | "cancel" }

function errorCode(error: unknown) {
  if ((error as { status?: number }).status === 401) return "BUYBACK_LOGIN_REQUIRED"
  return (error as Error).message?.match(/BUYBACK_[A-Z_]+/)?.[0] || "BUYBACK_REQUEST_FAILED"
}
async function call<T>(path: string, body?: unknown) {
  const headers = await getAuthHeaders()
  if (!headers.authorization) return { data: null, error: "BUYBACK_LOGIN_REQUIRED" }
  try {
    const data = await sdk.client.fetch<T>(path, { headers, cache: "no-store", ...(body ? { method: "POST", body: body as Record<string, unknown> } : {}) })
    return { data, error: null }
  } catch (error) { return { data: null, error: errorCode(error) } }
}
export type BuybackSummary = { recycled: number; processing: number; received: { currency_code: string; amount: number }[] }
export async function listBuybackRequests(offset = 0) { return call<{ requests: BuybackRequest[]; count: number; summary: BuybackSummary }>(`/store/buyback/requests?offset=${Math.max(0, offset)}`) }
export async function getBuybackRequest(id: string) { return call<{ request: BuybackRequest }>(`/store/buyback/requests/${encodeURIComponent(id)}`) }
export async function createBuybackRequest(body: BuybackDraft) { return call<{ request: BuybackRequest }>("/store/buyback/requests", body) }
export async function updateBuybackRequest(id: string, body: BuybackCommand) { return call<{ request: BuybackRequest }>(`/store/buyback/requests/${encodeURIComponent(id)}`, body) }
export async function uploadBuybackPhoto(id: string, body: { item_id: string; photo_type: string; mime_type: string; content: string }) { return call<{ photo: BuybackPhoto }>(`/store/buyback/requests/${encodeURIComponent(id)}/photos`, body) }
export async function getBuybackPhoto(id: string, photoId: string) { return call<{ content: string; mime_type: string }>(`/store/buyback/requests/${encodeURIComponent(id)}/photos/${encodeURIComponent(photoId)}`) }
export async function getBuybackReferencePrices(productId: string) {
  try {
    const { prices } = await sdk.client.fetch<{ prices: { capacity: string; amount: number; currency_code: string }[] }>(`/store/buyback/prices?product_id=${encodeURIComponent(productId)}`, { cache: "no-store" })
    return { prices, error: null }
  } catch { return { prices: [], error: "BUYBACK_REQUEST_FAILED" } }
}

export type BuybackEstimate = { amount: number | null; status: "estimated" | "legacy" | "unconfigured" | "disabled" | "ineligible"; provisional: boolean; currency_code: string }
export async function getBuybackEstimate(productId: string, capacity: string, assessment: BuybackAssessment) {
  try {
    const estimate = await sdk.client.fetch<BuybackEstimate>("/store/buyback/estimate", { method: "POST", body: { product_id: productId, capacity, assessment }, cache: "no-store" })
    return { estimate, error: null }
  } catch { return { estimate: null, error: "BUYBACK_REQUEST_FAILED" } }
}
