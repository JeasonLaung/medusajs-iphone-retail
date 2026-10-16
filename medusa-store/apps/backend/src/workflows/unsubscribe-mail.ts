import {
  createStep,
  createWorkflow,
  StepResponse,
  WorkflowResponse,
} from "@medusajs/framework/workflows-sdk"
import { acquireLockStep, releaseLockStep } from "@medusajs/medusa/core-flows"
import MailModuleService from "../modules/mail/service"
import { MAIL_MODULE } from "../modules/mail"

const unsubscribeMailStep = createStep(
  "unsubscribe-mail",
  async (input: { token: string }, { container }) => {
    const service: MailModuleService = container.resolve(MAIL_MODULE)
    const [contact] = await service.listMailContacts(
      { unsubscribe_token: input.token },
      { take: 1 }
    )
    if (contact)
      await service.updateMailContacts({ id: contact.id, subscribed: false })
    return new StepResponse({ ok: !!contact })
  }
)

export const unsubscribeMailWorkflow = createWorkflow(
  "unsubscribe-mail",
  function (input: { token: string }) {
    acquireLockStep({ key: "mail-operations", timeout: 30, ttl: 180 })
    const result = unsubscribeMailStep(input)
    releaseLockStep({ key: "mail-operations" })
    return new WorkflowResponse(result)
  }
)
