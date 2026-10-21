import { createCipheriv, createDecipheriv, createHash, randomBytes } from "node:crypto"
import { MedusaError } from "@medusajs/framework/utils"

function key() {
  const secret = process.env.MOLLIE_ENCRYPTION_KEY || process.env.COOKIE_SECRET
  if (!secret || secret === "supersecret") {
    throw new MedusaError(MedusaError.Types.INVALID_DATA, "MOLLIE_ENCRYPTION_KEY or a non-default COOKIE_SECRET is required")
  }
  return createHash("sha256").update(secret).digest()
}

export function encryptMollieSecret(value: string) {
  const iv = randomBytes(12)
  const cipher = createCipheriv("aes-256-gcm", key(), iv)
  const encrypted = Buffer.concat([cipher.update(value, "utf8"), cipher.final()])
  return [iv, cipher.getAuthTag(), encrypted].map((part) => part.toString("base64")).join(".")
}

export function decryptMollieSecret(value: string) {
  if (!value) return ""
  const [iv, tag, encrypted] = value.split(".").map((part) => Buffer.from(part, "base64"))
  const decipher = createDecipheriv("aes-256-gcm", key(), iv)
  decipher.setAuthTag(tag)
  return Buffer.concat([decipher.update(encrypted), decipher.final()]).toString("utf8")
}
