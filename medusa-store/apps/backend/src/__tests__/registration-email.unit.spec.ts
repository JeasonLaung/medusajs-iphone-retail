import { guardRegistrationVerification, guardVerifiedCustomerRegistration } from "../api/registration-guards"
import { registrationEmail, registrationMailAccount } from "../lib/registration-email"

const email = "registration-test@example.invalid"
const auth = {
  retrieveAuthIdentity: jest.fn(),
  listAuthVerifications: jest.fn(),
}
const mail = { listMailAccounts: jest.fn() }
const container = { resolve: (name: string) => name === "auth" ? auth : mail }
const req = (body: unknown) => ({
  scope: container,
  auth_context: { actor_type: "customer", auth_identity_id: "auth_test" },
  body,
}) as any
const response = { setHeader: jest.fn(), status: jest.fn().mockReturnThis(), json: jest.fn() } as any

beforeEach(() => {
  jest.clearAllMocks()
  auth.retrieveAuthIdentity.mockResolvedValue({ provider_identities: [{ provider: "emailpass", entity_id: email }] })
  auth.listAuthVerifications.mockResolvedValue([])
  mail.listMailAccounts.mockResolvedValue([{ id: "macc_test", api_key: "encrypted", enabled: true }])
  process.env.STOREFRONT_URL = "https://shop.example.invalid"
})

afterAll(() => { delete process.env.STOREFRONT_URL })

it("rejects creating a customer before email verification", async () => {
  const next = jest.fn()
  await expect(guardVerifiedCustomerRegistration(req({ email }), response, next)).rejects.toThrow("Verify your email")
  expect(next).not.toHaveBeenCalled()
})

it("only permits the verified email when creating the customer", async () => {
  auth.listAuthVerifications.mockResolvedValue([{ verified_at: new Date() }])
  const next = jest.fn()
  await expect(guardVerifiedCustomerRegistration(req({ email: "other@example.invalid" }), response, next)).rejects.toThrow("Verify your email")
  await guardVerifiedCustomerRegistration(req({ email }), response, next)
  expect(next).toHaveBeenCalledTimes(1)
})

it("rejects verification requests for somebody else's address", async () => {
  await expect(guardRegistrationVerification(req({ entity_id: "other@example.invalid", entity_type: "email" }), response, jest.fn())).rejects.toThrow("Only your own email")
})

it("fails clearly when no usable mail sender exists", async () => {
  mail.listMailAccounts.mockResolvedValue([])
  await expect(guardRegistrationVerification(req({ entity_id: email, entity_type: "email" }), response, jest.fn())).rejects.toThrow("not configured")
})

it("throttles repeated verification email requests", async () => {
  auth.listAuthVerifications.mockResolvedValue([{ requested_at: new Date() }])
  const next = jest.fn()
  await guardRegistrationVerification(req({ entity_id: email, entity_type: "email" }), response, next)
  expect(response.status).toHaveBeenCalledWith(429)
  expect(next).not.toHaveBeenCalled()
})

it("allows a new verification email after the cooldown", async () => {
  auth.listAuthVerifications.mockResolvedValue([{ requested_at: new Date(Date.now() - 61_000) }])
  const next = jest.fn()
  await guardRegistrationVerification(req({ entity_id: email, entity_type: "email" }), response, next)
  expect(next).toHaveBeenCalledTimes(1)
})

it("builds localized verification links using the configured storefront", () => {
  const result = registrationEmail({ entity_id: email, entity_type: "email", auth_identity_id: "auth_test", code: "abc&def", metadata: { locale: "zh", country: "de" } })
  expect(result.subject).toContain("验证邮箱")
  expect(result.text_body).toContain("https://shop.example.invalid/de-zh/verify-account?token=abc%26def")
  expect(result.text_body).toContain("15")
})

it("does not accept arbitrary redirect URLs from verification metadata", () => {
  const result = registrationEmail({ entity_id: email, entity_type: "email", auth_identity_id: "auth_test", code: "test", metadata: { country: "//evil.invalid", locale: "en", redirect_url: "https://evil.invalid" } })
  expect(result.text_body).toContain("https://shop.example.invalid/dk-en/verify-account")
  expect(result.text_body).not.toContain("evil.invalid")
})

it("selects a sender with an API key", async () => {
  mail.listMailAccounts.mockResolvedValue([{ id: "missing", api_key: "" }, { id: "ready", api_key: "encrypted" }])
  expect((await registrationMailAccount(container as any)).id).toBe("ready")
})
