import type { MedusaContainer } from "@medusajs/framework/types"
import { MedusaError } from "@medusajs/framework/utils"
import { MAIL_MODULE } from "../modules/mail"
import type MailModuleService from "../modules/mail/service"
import { escapeHtml } from "../modules/mail/security"

export type RegistrationVerification = {
  entity_id: string
  entity_type: string
  auth_identity_id: string
  code?: string
  expires_at?: string | Date
  metadata?: Record<string, unknown>
}

export async function registrationMailAccount(container: MedusaContainer) {
  const service: MailModuleService = container.resolve(MAIL_MODULE)
  const accounts = await service.listMailAccounts(
    { enabled: true, ...(process.env.REGISTRATION_MAIL_ACCOUNT_ID ? { id: process.env.REGISTRATION_MAIL_ACCOUNT_ID } : {}) },
    { order: { created_at: "ASC" } }
  )
  const account = accounts.find((item) => !!item.api_key)
  if (!account) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "Registration email sender is not configured")
  return account
}

export function registrationEmail(data: RegistrationVerification) {
  const base = process.env.STOREFRONT_URL || (process.env.NODE_ENV !== "production" ? "http://localhost:8000" : "")
  if (!/^https?:\/\//.test(base)) throw new Error("STOREFRONT_URL is required for registration emails")
  const locale = data.metadata?.locale === "zh" ? "zh" : "en"
  const country = typeof data.metadata?.country === "string" && /^[a-z]{2}$/.test(data.metadata.country) ? data.metadata.country : "dk"
  const url = new URL(`${base.replace(/\/+$/, "")}/${country}-${locale}/verify-account`)
  url.searchParams.set("token", data.code || "")
  const title = locale === "zh" ? "验证邮箱，完成 EuroRefurb 注册" : "Verify your email to join EuroRefurb"
  const intro = locale === "zh" ? "请点击下方链接验证邮箱，之后登录即可完成注册。" : "Verify your email using the link below, then sign in to complete your registration."
  const action = locale === "zh" ? "验证邮箱" : "Verify email"
  const expiry = locale === "zh" ? "链接在申请后 15 分钟内有效，且只能使用一次。如果你没有申请注册，请忽略此邮件。" : "This link expires 15 minutes after your request and can only be used once. If you did not request this, you can ignore this email."
  return {
    subject: title,
    text_body: `${intro}\n\n${url.toString()}\n\n${expiry}`,
    html_body: `<h1>${title}</h1><p>${intro}</p><p><a href="${escapeHtml(url.toString())}">${action}</a></p><p>${expiry}</p>`,
  }
}
