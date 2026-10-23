import type { AuthenticatedMedusaRequest, MedusaResponse, MedusaNextFunction } from "@medusajs/framework/http"
import type { IAuthModuleService } from "@medusajs/framework/types"
import { MedusaError, Modules } from "@medusajs/framework/utils"
import { registrationEmail, registrationMailAccount } from "../lib/registration-email"

async function customerEmail(req: AuthenticatedMedusaRequest) {
  const auth: IAuthModuleService = req.scope.resolve(Modules.AUTH)
  const identity = await auth.retrieveAuthIdentity(req.auth_context.auth_identity_id, { relations: ["provider_identities"] })
  const email = identity.provider_identities?.find((item) => item.provider === "emailpass")?.entity_id
  if (!email) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "Email identity is required")
  return { auth, email }
}

export async function guardRegistrationVerification(req: AuthenticatedMedusaRequest, res: MedusaResponse, next: MedusaNextFunction) {
  if (req.auth_context.actor_type !== "customer") return next()
  const { auth, email } = await customerEmail(req)
  const body = req.body as { entity_id?: string; entity_type?: string }
  if (body.entity_type !== "email" || body.entity_id !== email) {
    throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "Only your own email can be verified")
  }
  await registrationMailAccount(req.scope)
  registrationEmail({ entity_id: email, entity_type: "email", auth_identity_id: req.auth_context.auth_identity_id })
  const [verification] = await auth.listAuthVerifications({ auth_identity_id: req.auth_context.auth_identity_id, entity_id: email, entity_type: "email" })
  if (verification?.requested_at && Date.now() - new Date(verification.requested_at).getTime() < 60_000) {
    res.setHeader("Retry-After", "60")
    res.status(429).json({ message: "Please wait a minute before requesting another verification email" })
    return
  }
  next()
}

export async function guardVerifiedCustomerRegistration(req: AuthenticatedMedusaRequest, _res: MedusaResponse, next: MedusaNextFunction) {
  const { auth, email } = await customerEmail(req)
  const [verification] = await auth.listAuthVerifications({ auth_identity_id: req.auth_context.auth_identity_id, entity_id: email, entity_type: "email" })
  if (!verification?.verified_at || (req.body as { email?: string }).email !== email) {
    throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "Verify your email before completing registration")
  }
  next()
}
