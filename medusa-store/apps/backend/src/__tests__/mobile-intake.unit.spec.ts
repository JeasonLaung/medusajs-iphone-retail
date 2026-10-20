import { createIntakeToken, verifyIntakeToken } from "../lib/mobile-intake-token"
import { MobileIntakeBody } from "../api/mobile-intake/validators"

describe("temporary mobile intake authorization", () => {
  const secret = "mobile-intake-test-secret"
  const now = 1700000000000
  it("binds a two-hour session to its creator and warehouse", () => {
    const link = createIntakeToken("user_1", "source_1", secret, now)
    expect(link.expires_at).toBe(now + 7200000)
    expect(verifyIntakeToken(link.token, secret, now)).toMatchObject({ actor_id: "user_1", source_id: "source_1" })
    expect(() => verifyIntakeToken(link.token, secret, link.expires_at)).toThrow("MOBILE_INTAKE_LINK_INVALID")
  })
  it("rejects forged warehouses, wrong secrets and malformed tokens", () => {
    const { token } = createIntakeToken("user_1", "source_1", secret, now)
    const [payload, signature] = token.split(".")
    const session = JSON.parse(Buffer.from(payload, "base64url").toString())
    session.source_id = "source_2"
    const forged = `${Buffer.from(JSON.stringify(session)).toString("base64url")}.${signature}`
    for (const value of [forged, "", "invalid", token + ".extra", token.slice(0, -8)])
      expect(() => verifyIntakeToken(value, secret, now)).toThrow("MOBILE_INTAKE_LINK_INVALID")
    expect(() => verifyIntakeToken(token, "other-secret", now)).toThrow("MOBILE_INTAKE_LINK_INVALID")
    expect(() => createIntakeToken("user_1", "source_1", "", now)).toThrow()
  })
  it("accepts intake fields while rejecting warehouse and command injection", () => {
    const device = { serial_number: "serial123", variant_id: "variant_1", imei: "123456789012345" }
    expect(MobileIntakeBody.parse(device).serial_number).toBe("SERIAL123")
    expect(MobileIntakeBody.safeParse({ ...device, source_id: "source_2" }).success).toBe(false)
    expect(MobileIntakeBody.safeParse({ ...device, category: "sale" }).success).toBe(false)
    expect(MobileIntakeBody.safeParse({ ...device, action: "ship" }).success).toBe(false)
    expect(MobileIntakeBody.safeParse({ ...device, imei: "123" }).success).toBe(false)
    expect(MobileIntakeBody.safeParse({ ...device, serial_number: "" }).success).toBe(false)
  })
  it("binds the intake category to the signed session", () => {
    const { token } = createIntakeToken("user_1", "source_1", secret, now, "recycled")
    expect(verifyIntakeToken(token, secret, now).category).toBe("recycled")
    const [payload, signature] = token.split(".")
    const session = JSON.parse(Buffer.from(payload, "base64url").toString())
    session.category = "sale"
    expect(() => verifyIntakeToken(`${Buffer.from(JSON.stringify(session)).toString("base64url")}.${signature}`, secret, now)).toThrow()
  })
})
