import { createHmac, createHash } from "node:crypto"
import { Readable } from "node:stream"
import { RelaymeEvent, verifyRelayme, submitRelayme, relaymeDeliveryStatus, type RelaymeDelivery, type RelaymeInbound } from "../modules/mail/relayme"
import { encryptSecret, publicAccount } from "../modules/mail/security"
import { selectRelaymeMessage } from "../modules/mail/relayme-imap"
import { applyRelaymeEvent } from "../workflows/relayme-webhook"
import { receiveRelayme } from "../api/relayme-callback"

jest.mock("../modules/mail/relayme-imap", () => ({
  ...jest.requireActual("../modules/mail/relayme-imap"), fetchRelaymeInbound: jest.fn(),
}))
import { fetchRelaymeInbound } from "../modules/mail/relayme-imap"

const secret = "test-client-secret"
const inbound: RelaymeInbound = {
  event_id: "0d271fde-481a-4e8c-9264-fde40301a260", username: "alice@example.com",
  occurred_at: "2026-09-08T12:00:00Z", mailbox: "INBOX", from: "Sender <sender@example.com>",
  from_address: "sender@example.com", from_display_name: "Sender", to: "Alice <alice@example.com>",
  to_address: "alice@example.com", to_display_name: "Alice", subject: "主题",
}
const delivery: RelaymeDelivery = {
  event_id: "gateway-result-a", event_type: "delivery_status", submission_id: "submission-a",
  mail_from: "alice@example.com", message_id: "<a@example.com>", recipient: "one@example.net",
  status: "success", enhanced_code: "2.0.0", diagnostic: "", occurred_at: "2026-09-08T12:01:00Z",
}
const oldEncryptionKey = process.env.MAIL_ENCRYPTION_KEY
beforeAll(() => { process.env.MAIL_ENCRYPTION_KEY = "relayme-unit-test" })
afterAll(() => {
  if (oldEncryptionKey === undefined) delete process.env.MAIL_ENCRYPTION_KEY
  else process.env.MAIL_ENCRYPTION_KEY = oldEncryptionKey
})
afterEach(() => jest.restoreAllMocks())

function signed(rawBody: Buffer, path = "/api/callback/quguo_mail") {
  const timestamp = String(Math.floor(Date.now() / 1000))
  const nonce = "test-nonce"
  const signature = createHmac("sha256", secret).update(["POST", path, timestamp, nonce, createHash("sha256").update(rawBody).digest("hex")].join("\n")).digest("base64url")
  return { method: "POST", path, rawBody, timestamp, nonce, signature, secret }
}

it("verifies exact raw bytes and rejects stale, tampered, padded or wrong-path signatures", () => {
  const args = signed(Buffer.from('{ "subject": "主题" }'), "/api/callback/%71uguo_mail")
  expect(verifyRelayme(args)).toBe(true)
  for (const change of [{ rawBody: Buffer.from('{"subject":"主题"}') }, { path: "/api/callback/quguo_mail" }, { method: "GET" }, { secret: "wrong" }, { nonce: "" }, { signature: args.signature + "=" }, { timestamp: "1" }, { timestamp: "1e3" }])
    expect(verifyRelayme({ ...args, ...change })).toBe(false)
  expect(verifyRelayme({ ...args, now: Date.now() + 301000 })).toBe(false)
})

it("validates incoming and delivery event variants without inventing message bodies", () => {
  expect(RelaymeEvent.safeParse(inbound).success).toBe(true)
  expect(RelaymeEvent.safeParse(delivery).success).toBe(true)
  expect(RelaymeEvent.safeParse({ ...delivery, status: "accepted" }).success).toBe(false)
  expect(RelaymeEvent.safeParse({ ...inbound, occurred_at: "invalid" }).success).toBe(false)
})

it("sends documented fields and attachments, requires 201 acceptance and never automatically retries ambiguity", async () => {
  const request = jest.spyOn(global, "fetch").mockResolvedValue(new Response(JSON.stringify({ id: "submission-a", object: "email", status: "accepted", delivery_status: "awaiting_dsn" }), { status: 201 }))
  const account = { api_url: "https://postmaster.example.com", api_key: encryptSecret("id." + secret) }
  const payload = { from: "alice@example.com", to: ["one@example.net"], bcc: ["two@example.net"], replyTo: "reply@example.com", subject: "Hello", text: "Body", attachments: [{ filename: "a.txt", content: "YQ==" }] }
  expect(await submitRelayme(account, payload)).toEqual({ id: "submission-a", error: "", retry: false })
  const options = request.mock.calls[0][1]!
  expect(request.mock.calls[0][0]).toBe("https://postmaster.example.com/api/external/v1/emails")
  expect(JSON.parse(options.body as string)).toMatchObject({ bcc: payload.bcc, reply_to: payload.replyTo, attachments: payload.attachments })
  expect(options.redirect).toBe("error")
  request.mockRejectedValueOnce(new Error("timeout"))
  expect(await submitRelayme(account, payload)).toEqual({ id: null, retry: false, error: "relayme_submission_unknown" })
  request.mockResolvedValueOnce(new Response("invalid key", { status: 401 }))
  expect((await submitRelayme(account, payload)).error).toBe("relayme_rejected_401")
  request.mockResolvedValueOnce(new Response("{}", { status: 201 }))
  expect((await submitRelayme(account, payload)).error).toBe("relayme_submission_unknown")
})

it("requires all recipients to succeed and preserves terminal recipient results against delayed callbacks", () => {
  const recipients = ["one@example.net", "two@example.net"]
  expect(relaymeDeliveryStatus(recipients, [delivery])).toBe("sent")
  expect(relaymeDeliveryStatus(recipients, [delivery, { ...delivery, recipient: recipients[1] }])).toBe("delivered")
  expect(relaymeDeliveryStatus(recipients, [delivery, { ...delivery, recipient: recipients[1], status: "failed" }])).toBe("failed")
  expect(relaymeDeliveryStatus([recipients[0]], [delivery, { ...delivery, status: "delayed", occurred_at: "2026-09-08T12:02:00Z" }])).toBe("delivered")
})

it("rejects ambiguous IMAP matches and mismatched sender or recipient", () => {
  const candidate = { uid: 1, date: new Date(inbound.occurred_at), from: [inbound.from_address], to: [inbound.to_address], subject: inbound.subject }
  expect(selectRelaymeMessage([candidate], inbound)).toBe(1)
  expect(() => selectRelaymeMessage([candidate, { ...candidate, uid: 2 }], inbound)).toThrow("imapAmbiguous")
  expect(() => selectRelaymeMessage([{ ...candidate, from: ["other@example.com"] }], inbound)).toThrow("imapMessageNotFound")
  expect(() => selectRelaymeMessage([{ ...candidate, to: ["other@example.com"] }], inbound)).toThrow("imapMessageNotFound")
})

it("never exposes IMAP passwords or provider secrets", () => {
  const safe = publicAccount({ api_key: "secret", webhook_secret: "secret", imap_password: "password", id: "account" })
  expect(safe).toEqual({ id: "account", has_api_key: true, has_webhook_secret: true, has_imap_password: true })
})

function fakeService() {
  const events: any[] = [], messages: any[] = []
  return {
    events, messages,
    listMailEvents: jest.fn(async (filter) => events.filter((e) => Object.entries(filter).every(([k, v]) => e[k] === v))),
    listMailAccounts: jest.fn(async () => [{ id: "account", email: "alice@example.com" }]),
    listMailMessages: jest.fn(async (filter) => messages.filter((m) => Object.entries(filter).every(([k, v]) => m[k] === v))),
    createMailMessages: jest.fn(async (m) => { messages.push(m); return m }),
    createMailEvents: jest.fn(async (e) => { events.push(e); return e }),
    listMailDeliveries: jest.fn(async () => []),
    updateMailMessages: jest.fn(async () => ({})),
  }
}

it("persists deduplication only after inbound processing and safely retries a failed attempt", async () => {
  const service = fakeService()
  const input = { account_ids: ["account"], key_id: "key", event: inbound }
  const fetchInbound = fetchRelaymeInbound as jest.Mock
  fetchInbound.mockRejectedValueOnce(new Error("imap down"))
  await expect(applyRelaymeEvent(input, service as any)).rejects.toThrow("imap down")
  expect(service.events).toHaveLength(0)
  fetchInbound.mockResolvedValue({ provider_id: "stable-imap-id", text_body: "正文", attachments: [] })
  await applyRelaymeEvent(input, service as any)
  await applyRelaymeEvent(input, service as any)
  expect(service.messages).toHaveLength(1)
  expect(service.events).toHaveLength(1)
  expect(service.messages[0]).toMatchObject({ folder: "inbox", read: false, text_body: "正文" })
})

it("scopes delivery callbacks to the account and recipient, rejecting unknown submissions", async () => {
  const service = fakeService()
  const input = { account_ids: ["account"], key_id: "key", event: delivery }
  await expect(applyRelaymeEvent(input, service as any)).rejects.toThrow("relaymeSubmissionNotFound")
  service.messages.push({ id: "message", account_id: "account", provider_id: delivery.submission_id, direction: "outbound", to: [delivery.recipient], cc: [], bcc: [] })
  await applyRelaymeEvent(input, service as any)
  expect(service.updateMailMessages).toHaveBeenCalledWith(expect.objectContaining({ id: "message", status: "delivered" }))
  await applyRelaymeEvent(input, service as any)
  expect(service.events).toHaveLength(1)
  await expect(applyRelaymeEvent({ ...input, event: { ...delivery, event_id: "other", mail_from: "stranger@example.com" } }, service as any)).rejects.toThrow("relaymeMailboxMismatch")
})

it("rejects invalid raw JSON before parsing when unsigned, and enforces signed event ID equality", async () => {
  const account = { id: "account", webhook_secret: encryptSecret(secret) }
  async function call(raw: string, sign: boolean, eventId: string) {
    const body = Buffer.from(raw), args = signed(body)
    const headers = { "X-Relayme-Key-ID": "key", "X-Relayme-Timestamp": args.timestamp, "X-Relayme-Nonce": args.nonce, "X-Relayme-Signature": sign ? args.signature : "bad", "X-Relayme-Event-ID": eventId }
    const req = Object.assign(Readable.from([body]), { method: "POST", originalUrl: args.path + "?ignored=1", get: (key: string) => headers[key], scope: { resolve: () => ({ listMailAccounts: async () => [account] }) } })
    const res = { sendStatus: jest.fn() }
    await receiveRelayme(req as any, res as any, "inbound")
    return res.sendStatus.mock.calls[0][0]
  }
  expect(await call("not-json", false, "")).toBe(401)
  expect(await call("not-json", true, "")).toBe(400)
  expect(await call(JSON.stringify(inbound), true, "wrong-event-id")).toBe(400)
})
