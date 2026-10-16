import { Resend } from "resend"
import {
  decryptSecret,
  encryptSecret,
  publicAccount,
} from "../modules/mail/security"
import { MailCommand } from "../api/admin/mail/validators"
import { deliveryStatus } from "../workflows/mail-webhook"
import { submitMail } from "../workflows/steps/process-mail-queue"

describe("mail delivery safety", () => {
  const encryptionKey = process.env.MAIL_ENCRYPTION_KEY
  beforeAll(() => {
    process.env.MAIL_ENCRYPTION_KEY = "unit-test-encryption-key"
  })
  afterAll(() => {
    if (encryptionKey === undefined) delete process.env.MAIL_ENCRYPTION_KEY
    else process.env.MAIL_ENCRYPTION_KEY = encryptionKey
  })

  it("encrypts credentials using a fresh IV and rejects tampered ciphertext", () => {
    const encrypted = encryptSecret("re_test_secret")
    expect(encrypted).not.toContain("re_test_secret")
    expect(encryptSecret("re_test_secret")).not.toBe(encrypted)
    expect(decryptSecret(encrypted)).toBe("re_test_secret")
    const parts = encrypted.split(".")
    parts[2] = Buffer.from("tampered").toString("base64")
    expect(() => decryptSecret(parts.join("."))).toThrow()
    expect(
      publicAccount({
        id: "macc_test",
        api_key: encrypted,
        webhook_secret: encrypted,
      })
    ).toEqual({ id: "macc_test", has_api_key: true, has_webhook_secret: true })
  })

  it("rejects header injection, invalid addresses and oversized attachments", () => {
    const input = {
      action: "save_draft",
      account_id: "macc_test",
      to: ["client@example.com"],
      subject: "Hello",
    }
    expect(MailCommand.safeParse(input).success).toBe(true)
    expect(
      MailCommand.safeParse({
        ...input,
        subject: "Hello\r\nBcc: victim@example.com",
      }).success
    ).toBe(false)
    expect(
      MailCommand.safeParse({ ...input, to: ["not-an-email"] }).success
    ).toBe(false)
    expect(
      MailCommand.safeParse({
        ...input,
        attachments: [
          { filename: "a", content: "A".repeat(8_000_000) },
          { filename: "b", content: "A".repeat(8_000_000) },
        ],
      }).success
    ).toBe(false)
    expect(
      MailCommand.safeParse({
        action: "save_account",
        name: "bad\r\nFrom: other",
        email: "mail@example.com",
        enabled: true,
      }).success
    ).toBe(false)
  })

  it("never regresses delivered or failed state for out-of-order callbacks", () => {
    expect(deliveryStatus("delivered", "sent")).toBe("delivered")
    expect(deliveryStatus("bounced", "delivered")).toBe("bounced")
    expect(deliveryStatus("sent", "bounced")).toBe("bounced")
    expect(deliveryStatus("delivered", "opened")).toBe("delivered")
  })

  it("uses the same provider idempotency key on retries and distinguishes permanent failures", async () => {
    const send = jest
      .fn()
      .mockResolvedValueOnce({
        data: null,
        error: { name: "rate_limit_exceeded" },
      })
      .mockResolvedValueOnce({ data: { id: "provider_test" }, error: null })
      .mockResolvedValueOnce({
        data: null,
        error: { name: "validation_error" },
      })
    const resend = { emails: { send } } as unknown as Resend
    const payload = {
      from: "mail@example.com",
      to: "client@example.com",
      subject: "Test",
      text: "Test",
    }
    expect((await submitMail(resend, "mail_test", payload)).retry).toBe(true)
    expect((await submitMail(resend, "mail_test", payload)).id).toBe(
      "provider_test"
    )
    expect(send.mock.calls[0][1].idempotencyKey).toBe(
      send.mock.calls[1][1].idempotencyKey
    )
    expect((await submitMail(resend, "mail_test", payload)).retry).toBe(false)
  })
})
