"use server"

import { sdk } from "@lib/config"
import { getAuthHeaders } from "./cookies"

export type CustomerNotice = { id: string; kind: string; resource_id: string; reference: string; action: string; read_at: string | null; created_at: string }
export type NoticePage = { notifications: CustomerNotice[]; count: number; unread: number }
export async function listNotifications(offset = 0) {
  const headers = await getAuthHeaders()
  if (!headers.authorization) return { data: null, error: "login" }
  try {
    return { data: await sdk.client.fetch<NoticePage>(`/store/notifications?offset=${Math.max(0, offset)}`, { headers, cache: "no-store" }), error: null }
  } catch { return { data: null, error: "request" } }
}
export async function readNotifications(id?: string) {
  const headers = await getAuthHeaders()
  if (!headers.authorization) return false
  try {
    await sdk.client.fetch("/store/notifications", { method: "POST", headers, body: id ? { id } : {}, cache: "no-store" })
    return true
  } catch { return false }
}
