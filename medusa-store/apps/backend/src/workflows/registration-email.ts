import { createHash } from "node:crypto"
import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { acquireLockStep, releaseLockStep } from "@medusajs/medusa/core-flows"
import { registrationEmail, registrationMailAccount, type RegistrationVerification } from "../lib/registration-email"
import { MAIL_MODULE } from "../modules/mail"
import type MailModuleService from "../modules/mail/service"
import { jsonArray } from "../modules/mail/types"

const queueRegistrationEmailStep = createStep("queue-registration-email", async (input: RegistrationVerification, { container }) => {
  if (input.entity_type !== "email" || !input.code || !input.expires_at || new Date(input.expires_at).getTime() <= Date.now()) return new StepResponse(null)
  const service: MailModuleService = container.resolve(MAIL_MODULE)
  const account = await registrationMailAccount(container)
  // Stable ID prevents duplicate outbox messages when the event is retried.
  const id = `mail_verify_${createHash("sha256").update(input.code).digest("hex")}`
  const [existing] = await service.listMailMessages({ id }, { take: 1 })
  if (existing) return new StepResponse({ id: existing.id })
  const message = await service.createMailMessages({
    id, account_id: account.id, folder: "outbox", direction: "outbound", status: "queued",
    from_address: `${account.name} <${account.email}>`, to: jsonArray([input.entity_id]),
    ...registrationEmail(input), created_by: "registration-verification",
  })
  return new StepResponse({ id: message.id })
})

export const registrationEmailWorkflow = createWorkflow("registration-email", function (input: RegistrationVerification) {
  acquireLockStep({ key: "registration-email", timeout: 10, ttl: 30 })
  const result = queueRegistrationEmailStep(input)
  releaseLockStep({ key: "registration-email" })
  return new WorkflowResponse(result)
})
