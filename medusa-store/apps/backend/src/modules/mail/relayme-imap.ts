import { createHash } from "node:crypto"
import { ImapFlow } from "imapflow"
import { simpleParser } from "mailparser"
import { MedusaError } from "@medusajs/framework/utils"
import { decryptSecret } from "./security"
import { jsonArray } from "./types"
import type { RelaymeInbound } from "./relayme"

type ImapAccount = {
  id: string; email: string; imap_host: string; imap_port: number
  imap_username: string; imap_password: string; imap_tls: string
}
const unavailable = (code: string) => new MedusaError(MedusaError.Types.UNEXPECTED_STATE, `mail.${code}`)

export function selectRelaymeMessage(
  candidates: { uid: number; date: Date; from: string[]; to: string[]; subject: string }[],
  event: RelaymeInbound
) {
  const matches = candidates.filter((m) =>
    event.from_address && m.from.some((a) => a.toLowerCase() === event.from_address.toLowerCase()) &&
    (!event.to_address || m.to.some((a) => a.toLowerCase() === event.to_address.toLowerCase())) &&
    m.subject === event.subject && Math.abs(m.date.getTime() - Date.parse(event.occurred_at)) <= 30 * 60_000
  )
  if (matches.length !== 1) throw unavailable(matches.length ? "imapAmbiguous" : "imapMessageNotFound")
  return matches[0].uid
}

export async function fetchRelaymeInbound(account: ImapAccount, event: RelaymeInbound) {
  if (!account.imap_host || !account.imap_username || !account.imap_password) throw unavailable("imapRequired")
  const client = new ImapFlow({
    host: account.imap_host, port: account.imap_port,
    secure: account.imap_tls === "tls",
    ...(account.imap_tls === "starttls" ? { doSTARTTLS: true } : {}),
    auth: { user: account.imap_username, pass: decryptSecret(account.imap_password) },
    tls: { minVersion: "TLSv1.2", rejectUnauthorized: true },
    logger: false, connectionTimeout: 8000, greetingTimeout: 8000, socketTimeout: 15000,
    disableAutoIdle: true,
  })
  // A slow or malicious IMAP server cannot hold the global mail lock indefinitely.
  const deadline = setTimeout(() => client.close(), 25000)
  try {
    await client.connect()
    const mailbox = await client.mailboxOpen(event.mailbox, { readOnly: true })
    const at = Date.parse(event.occurred_at)
    const ids = await client.search({ since: new Date(at - 48 * 3600_000), before: new Date(at + 48 * 3600_000), ...(event.from_address ? { from: event.from_address } : {}) }, { uid: true })
    if (!ids || !ids.length) throw unavailable("imapMessageNotFound")
    if (ids.length > 200) throw unavailable("imapAmbiguous")
    const candidates: Parameters<typeof selectRelaymeMessage>[0] = []
    for await (const item of client.fetch(ids, { envelope: true, internalDate: true }, { uid: true })) {
      const envelope = item.envelope
      candidates.push({ uid: item.uid, date: new Date(item.internalDate!),
        from: (envelope?.from || []).map((a) => a.address || ""),
        to: [...(envelope?.to || []), ...(envelope?.cc || []), ...(envelope?.bcc || [])].map((a) => a.address || ""),
        subject: envelope?.subject || "" })
    }
    const uid = selectRelaymeMessage(candidates, event)
    const meta = await client.fetchOne(uid, { size: true }, { uid: true })
    const maxSize = 25 * 1024 * 1024
    if (!meta || !meta.size || meta.size > maxSize) throw unavailable("imapMessageTooLarge")
    const downloaded = await client.download(uid, undefined, { uid: true, maxBytes: maxSize + 1 })
    if (!downloaded.content) throw unavailable("imapMessageNotFound")
    const chunks: Buffer[] = []
    let size = 0
    for await (const chunk of downloaded.content) {
      size += chunk.length
      if (size > maxSize) throw unavailable("imapMessageTooLarge")
      chunks.push(Buffer.from(chunk))
    }
    const parsed = await simpleParser(Buffer.concat(chunks), { skipImageLinks: true, skipHtmlToText: true, skipTextToHtml: true })
    const addresses = (value: typeof parsed.to) => (Array.isArray(value) ? value : value ? [value] : []).flatMap((v) => v.value.map((a) => a.address || "")).filter(Boolean)
    return {
      provider_id: `relayme-imap:${createHash("sha256").update(`${account.id}:${event.mailbox}:${mailbox.uidValidity}:${uid}`).digest("hex")}`,
      from_address: parsed.from?.text || event.from,
      to: jsonArray(addresses(parsed.to)), cc: jsonArray(addresses(parsed.cc)),
      bcc: jsonArray([]), reply_to: jsonArray(addresses(parsed.replyTo)),
      subject: parsed.subject || "", text_body: parsed.text || "", html_body: parsed.html || "",
      message_id: parsed.messageId || null, in_reply_to: parsed.inReplyTo || null,
      attachments: jsonArray(parsed.attachments.map((a, i) => ({ id: String(i), filename: a.filename || `attachment-${i + 1}`, size: a.size, content: a.content.toString("base64") }))),
    }
  } finally {
    clearTimeout(deadline)
    client.close()
  }
}
