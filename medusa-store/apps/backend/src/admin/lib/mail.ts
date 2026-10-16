import Medusa from "@medusajs/js-sdk"

export const mailSdk = new Medusa({
  baseUrl: import.meta.env.VITE_BACKEND_URL || "/",
  debug: import.meta.env.DEV,
  auth: { type: "session" },
})
export const mailGet = <T>(
  query: Record<string, string | number | undefined>
) => mailSdk.client.fetch<T>("/admin/mail", { query })
export const mailCommand = <T = { id?: string; ok?: boolean }>(
  body: Record<string, unknown>
) => mailSdk.client.fetch<T>("/admin/mail/commands", { method: "POST", body })
export const mailKey = ["mail"]
export type Account = {
  id: string
  name: string
  email: string
  reply_to: string
  enabled: boolean
  has_api_key: boolean
  has_webhook_secret: boolean
  webhook_url?: string
}
export type Attachment = {
  id?: string
  filename: string
  content?: string
  size?: number
}
export type Message = {
  id: string
  account_id: string
  folder: string
  direction: string
  from_address: string
  to: string[]
  cc: string[]
  bcc: string[]
  reply_to: string[]
  subject: string
  text_body: string
  html_body: string
  read: boolean
  starred: boolean
  status: string
  error: string
  created_at: string
  sent_at: string | null
  received_at: string | null
  message_id: string | null
  in_reply_to: string | null
  attachments: Attachment[]
}
export type Contact = {
  id: string
  email: string
  name: string
  subscribed: boolean
  suppressed: boolean
  consent_source: string
}
export type Campaign = {
  id: string
  account_id: string
  name: string
  subject: string
  text_body: string
  html_body: string
  status: string
  stats: Record<string, number>
}
export type Template = {
  id: string
  name: string
  subject: string
  text_body: string
  html_body: string
}
export type Delivery = {
  id: string
  campaign_id: string
  email: string
  status: string
  error: string
  attempts: number
  sent_at: string | null
  opened_at: string | null
  clicked_at: string | null
}
export const folders = [
  "inbox",
  "draft",
  "outbox",
  "sent",
  "archive",
  "trash",
] as const

export function mailError(error: unknown, translate: (key: string) => string) {
  const message = error instanceof Error ? error.message : ""
  const key = message.match(/mail\.[A-Za-z]+/)?.[0]
  return translate(key || "mail.requestFailed")
}

export function emailDocument(html: string, externalImages = false) {
  // The sandbox has no scripts, forms, navigation or same-origin access. CSP blocks tracking images by default.
  return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data: ${externalImages ? "https:" : ""}; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><style>body{font:14px/1.6 system-ui;overflow-wrap:anywhere;margin:20px;color:#222;background:#fff}img{max-width:100%;height:auto}table{max-width:100%}pre{white-space:pre-wrap}a{color:#315ccf}</style></head><body>${html}</body></html>`
}

export function recipientList(value: string) {
  return value
    .split(/[,;\n]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}
