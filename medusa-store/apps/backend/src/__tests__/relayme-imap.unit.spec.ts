import { Readable } from "node:stream"
import { ImapFlow } from "imapflow"
import { fetchRelaymeInbound } from "../modules/mail/relayme-imap"
import { encryptSecret } from "../modules/mail/security"
import type { RelaymeInbound } from "../modules/mail/relayme"

jest.mock("imapflow", () => ({ ImapFlow: jest.fn() }))

const event: RelaymeInbound = {
  event_id: "0d271fde-481a-4e8c-9264-fde40301a260", username: "alice@example.com",
  occurred_at: "2026-09-08T12:00:00Z", mailbox: "INBOX", from: "Sender <sender@example.com>",
  from_address: "sender@example.com", from_display_name: "Sender", to: "Alice <alice@example.com>",
  to_address: "alice@example.com", to_display_name: "Alice", subject: "主题",
}
const source = Buffer.from([
  "From: Sender <sender@example.com>", "To: Alice <alice@example.com>",
  "Subject: =?UTF-8?B?5Li76aKY?=", "Message-ID: <source@example.com>",
  "Reply-To: reply@example.com", "MIME-Version: 1.0", "Content-Type: multipart/mixed; boundary=sample", "",
  "--sample", "Content-Type: text/plain; charset=utf-8", "Content-Transfer-Encoding: base64", "", Buffer.from("邮件正文").toString("base64"),
  "--sample", "Content-Type: application/octet-stream", "Content-Disposition: attachment; filename=sample.txt",
  "Content-Transfer-Encoding: base64", "", Buffer.from("attachment bytes").toString("base64"), "--sample--", "",
].join("\r\n"))

const candidate = { uid: 42, internalDate: new Date(event.occurred_at), envelope: { from: [{ address: "sender@example.com" }], to: [{ address: "alice@example.com" }], subject: "主题" } }

it("fetches MIME bodies and attachments with readonly IMAP, encrypted auth, stable identity and verified TLS", async () => {
  const previous = process.env.MAIL_ENCRYPTION_KEY
  process.env.MAIL_ENCRYPTION_KEY = "imap-fixture-key"
  const client = {
    connect: jest.fn(async () => {}), close: jest.fn(),
    mailboxOpen: jest.fn(async () => ({ uidValidity: 7n })), search: jest.fn(async () => [42]),
    fetch: jest.fn(async function* () { yield candidate }),
    fetchOne: jest.fn(async () => ({ size: source.length })),
    download: jest.fn(async () => ({ content: Readable.from([source]) })),
  }
  ;(ImapFlow as unknown as jest.Mock).mockImplementation(() => client)
  const account = { id: "account", email: "alice@example.com", imap_host: "imap.example.com", imap_port: 993,
    imap_username: "alice@example.com", imap_password: encryptSecret("imap-password"), imap_tls: "tls" }
  try {
    const result = await fetchRelaymeInbound(account, event)
    expect(result).toMatchObject({ subject: "主题", text_body: "邮件正文", message_id: "<source@example.com>", reply_to: ["reply@example.com"] })
    expect(result.attachments).toEqual([expect.objectContaining({ filename: "sample.txt", content: Buffer.from("attachment bytes").toString("base64") })])
    expect(client.mailboxOpen).toHaveBeenCalledWith("INBOX", { readOnly: true })
    expect(client.download).toHaveBeenCalledWith(42, undefined, expect.objectContaining({ uid: true }))
    expect(ImapFlow).toHaveBeenCalledWith(expect.objectContaining({ secure: true, logger: false, auth: { user: account.imap_username, pass: "imap-password" }, tls: { minVersion: "TLSv1.2", rejectUnauthorized: true } }))
    const repeated = await fetchRelaymeInbound(account, event)
    expect(repeated.provider_id).toBe(result.provider_id)
    expect(client.close).toHaveBeenCalledTimes(2)
    client.fetch.mockImplementation(async function* () { yield candidate; yield { ...candidate, uid: 43 } })
    await expect(fetchRelaymeInbound(account, event)).rejects.toThrow("imapAmbiguous")
    expect(client.download).toHaveBeenCalledTimes(2)
    expect(client.close).toHaveBeenCalledTimes(3)
  } finally {
    if (previous === undefined) delete process.env.MAIL_ENCRYPTION_KEY
    else process.env.MAIL_ENCRYPTION_KEY = previous
  }
})
