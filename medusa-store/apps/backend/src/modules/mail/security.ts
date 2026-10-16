import {
  createCipheriv,
  createDecipheriv,
  createHash,
  randomBytes,
} from "node:crypto"
import { MedusaError } from "@medusajs/framework/utils"

function key() {
  const secret = process.env.MAIL_ENCRYPTION_KEY || process.env.COOKIE_SECRET
  if (!secret || secret === "supersecret")
    throw new MedusaError(
      MedusaError.Types.INVALID_DATA,
      "mail.encryptionRequired"
    )
  return createHash("sha256").update(secret).digest()
}

export function encryptSecret(value: string) {
  const iv = randomBytes(12)
  const cipher = createCipheriv("aes-256-gcm", key(), iv)
  const encrypted = Buffer.concat([
    cipher.update(value, "utf8"),
    cipher.final(),
  ])
  return [iv, cipher.getAuthTag(), encrypted]
    .map((part) => part.toString("base64"))
    .join(".")
}

export function decryptSecret(value: string) {
  if (!value) return ""
  const [iv, tag, encrypted] = value
    .split(".")
    .map((part) => Buffer.from(part, "base64"))
  const decipher = createDecipheriv("aes-256-gcm", key(), iv)
  decipher.setAuthTag(tag)
  return Buffer.concat([decipher.update(encrypted), decipher.final()]).toString(
    "utf8"
  )
}

export function publicAccount<
  T extends { api_key: string; webhook_secret: string },
>(account: T) {
  const { api_key, webhook_secret, ...safe } = account
  return {
    ...safe,
    has_api_key: !!api_key,
    has_webhook_secret: !!webhook_secret,
  }
}

export function escapeHtml(value: string) {
  return value.replace(
    /[&<>"']/g,
    (char) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        char
      ]!
  )
}

export function publicMailUrl(path: string) {
  const base = process.env.MAIL_PUBLIC_URL || process.env.MEDUSA_BACKEND_URL
  if (!base || !/^https?:\/\//.test(base))
    throw new MedusaError(
      MedusaError.Types.INVALID_DATA,
      "mail.publicUrlRequired"
    )
  return `${base.replace(/\/$/, "")}${path}`
}
