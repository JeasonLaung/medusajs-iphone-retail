import { MedusaError } from "@medusajs/framework/utils"
import { createCipheriv, createDecipheriv, createHash, randomBytes } from "node:crypto"

function key() {
  const value = process.env.WEBHOOK_ENCRYPTION_KEY || process.env.COOKIE_SECRET
  if (!value || value === "supersecret") throw new MedusaError(MedusaError.Types.INVALID_DATA, "webhooks.errors.encryption")
  return createHash("sha256").update(value).digest()
}
export function encrypt(value: string) {
  if (!value) return ""
  const iv = randomBytes(12)
  const cipher = createCipheriv("aes-256-gcm", key(), iv)
  const data = Buffer.concat([cipher.update(value, "utf8"), cipher.final()])
  return [iv, cipher.getAuthTag(), data].map(v => v.toString("base64")).join(".")
}
export function decrypt(value: string) {
  if (!value) return ""
  const [iv, tag, data] = value.split(".").map(v => Buffer.from(v, "base64"))
  const cipher = createDecipheriv("aes-256-gcm", key(), iv)
  cipher.setAuthTag(tag)
  return Buffer.concat([cipher.update(data), cipher.final()]).toString("utf8")
}
