import { createHmac, randomUUID, timingSafeEqual } from "node:crypto"
import { MedusaError } from "@medusajs/framework/utils"

export type IntakeSession = {
  purpose: "mobile-device-intake"
  actor_id: string
  category: "sale" | "returned" | "recycled"
  source_id: string
  expires_at: number
  nonce: string
}

const denied = () => new MedusaError(MedusaError.Types.UNAUTHORIZED, "MOBILE_INTAKE_LINK_INVALID")
const signature = (payload: string, secret: string) =>
  createHmac("sha256", secret).update(`mobile-intake:${payload}`).digest()

export function createIntakeToken(actorId: string, sourceId: string, secret: string, now = Date.now(), category: IntakeSession["category"] = "sale") {
  if (!secret) throw denied()
  const session: IntakeSession = {
    purpose: "mobile-device-intake", actor_id: actorId, source_id: sourceId, category,
    expires_at: now + 2 * 60 * 60 * 1000, nonce: randomUUID(),
  }
  const payload = Buffer.from(JSON.stringify(session)).toString("base64url")
  return { token: `${payload}.${signature(payload, secret).toString("base64url")}`, expires_at: session.expires_at }
}

export function verifyIntakeToken(token: string, secret: string, now = Date.now()): IntakeSession {
  if (!secret || !token || token.length > 2048) throw denied()
  const parts = token.split(".")
  if (parts.length !== 2) throw denied()
  const actual = Buffer.from(parts[1], "base64url")
  const expected = signature(parts[0], secret)
  if (actual.length !== expected.length || !timingSafeEqual(actual, expected)) throw denied()
  let session: IntakeSession
  try { session = JSON.parse(Buffer.from(parts[0], "base64url").toString()) } catch { throw denied() }
  if (session.purpose !== "mobile-device-intake" || !session.actor_id || !session.source_id ||
    !Number.isFinite(session.expires_at) || session.expires_at <= now) throw denied()
  session.category ??= "sale"
  if (!["sale", "returned", "recycled"].includes(session.category)) throw denied()
  return session
}
