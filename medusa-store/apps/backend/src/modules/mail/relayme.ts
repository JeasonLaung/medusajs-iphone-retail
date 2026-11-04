import { createHash, createHmac, timingSafeEqual } from "node:crypto"
import { z } from "@medusajs/framework/zod"
import { MedusaError } from "@medusajs/framework/utils"
import type { CreateEmailOptions } from "resend"
import { decryptSecret } from "./security"

export function relaymeOrigin(value: string) {
  const url = new URL(value)
  if (!["https:", "http:"].includes(url.protocol) || url.username || url.password || url.search || url.hash || url.pathname !== "/")
    throw new MedusaError(MedusaError.Types.INVALID_DATA, "mail.relaymeUrlRequired")
  return url.origin
}

export function verifyRelayme(input: {
  method: string; path: string; rawBody: Buffer; timestamp: string
  nonce: string; signature: string; secret: string; now?: number
}) {
  const { method, path, rawBody, timestamp, nonce, signature, secret } = input
  if (!secret || !/^\d+$/.test(timestamp) || !nonce.trim() || !/^[A-Za-z0-9_-]{43}$/.test(signature)) return false
  const seconds = Number(timestamp)
  if (!Number.isSafeInteger(seconds) || Math.abs(Math.floor((input.now ?? Date.now()) / 1000) - seconds) > 300) return false
  const hash = createHash("sha256").update(rawBody).digest("hex")
  const expected = createHmac("sha256", secret).update([method, path, timestamp, nonce, hash].join("\n")).digest()
  const received = Buffer.from(signature, "base64url")
  return received.length === expected.length && received.toString("base64url") === signature && timingSafeEqual(expected, received)
}

const common = {
  event_id: z.string().min(1).max(255),
  occurred_at: z.iso.datetime({ offset: true }),
}
export const RelaymeEvent = z.union([
  z.object({
    ...common,
    event_type: z.literal("delivery_status"),
    submission_id: z.string().min(1).max(255),
    mail_from: z.email(),
    message_id: z.string().min(1).max(998).regex(/^[^\r\n]*$/),
    recipient: z.email(),
    status: z.enum(["success", "delayed", "failed"]),
    smtp_code: z.number().int().optional(),
    enhanced_code: z.string().max(255),
    diagnostic: z.string().max(10000),
  }),
  z.object({
    ...common,
    event_id: z.uuid(),
    event_type: z.literal("").optional(),
    username: z.email(),
    mailbox: z.string().min(1).max(255).regex(/^[^\r\n\0]*$/),
    from: z.string().max(2000),
    from_address: z.union([z.email(), z.literal("")]),
    from_display_name: z.string().max(1000),
    to: z.string().max(2000),
    to_address: z.union([z.email(), z.literal("")]),
    to_display_name: z.string().max(1000),
    subject: z.string().max(10000),
  }),
])
export type RelaymeEventBody = z.infer<typeof RelaymeEvent>
export type RelaymeInbound = Extract<RelaymeEventBody, { username: string }>
export type RelaymeDelivery = Extract<RelaymeEventBody, { submission_id: string }>

export async function submitRelayme(
  account: { api_url: string; api_key: string },
  payload: CreateEmailOptions
) {
  try {
    const response = await fetch(`${relaymeOrigin(account.api_url)}/api/external/v1/emails`, {
      method: "POST",
      redirect: "error",
      headers: { Authorization: `Bearer ${decryptSecret(account.api_key)}`, "Content-Type": "application/json" },
      body: JSON.stringify({
        from: payload.from, to: payload.to, cc: payload.cc, bcc: payload.bcc,
        reply_to: payload.replyTo, subject: payload.subject, text: payload.text,
        html: payload.html, headers: payload.headers,
        attachments: payload.attachments?.map((a) => ({ filename: a.filename, content: a.content })),
      }),
      signal: AbortSignal.timeout(8000),
    })
    if (response.status !== 201) {
      // No documented idempotency contract. Only an explicit rejection is safe to retry manually.
      const rejected = response.status >= 400 && response.status < 500 && response.status !== 408
      await response.body?.cancel()
      return { id: null, error: rejected ? `relayme_rejected_${response.status}` : "relayme_submission_unknown", retry: false }
    }
    const data = await response.json()
    if (typeof data.id !== "string" || !data.id || data.object !== "email" || data.status !== "accepted" || data.delivery_status !== "awaiting_dsn")
      return { id: null, error: "relayme_submission_unknown", retry: false }
    return { id: data.id as string, error: "", retry: false }
  } catch {
    return { id: null, error: "relayme_submission_unknown", retry: false }
  }
}

// A submission may have multiple To/Cc/Bcc recipients. One success is not full delivery.
export function relaymeDeliveryStatus(recipients: string[], events: RelaymeDelivery[]) {
  const states = new Map<string, string>()
  for (const event of [...events].sort((a, b) => Date.parse(a.occurred_at) - Date.parse(b.occurred_at))) {
    const recipient = event.recipient.toLowerCase()
    if (["success", "failed"].includes(states.get(recipient) || "")) continue
    states.set(recipient, event.status)
  }
  const values = recipients.map((r) => states.get(r.toLowerCase()))
  if (values.includes("failed")) return "failed"
  if (values.length && values.every((s) => s === "success")) return "delivered"
  if (values.includes("delayed")) return "delivery_delayed"
  return "sent"
}
