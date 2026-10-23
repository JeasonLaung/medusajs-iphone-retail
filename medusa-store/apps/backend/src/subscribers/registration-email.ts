import type { SubscriberArgs, SubscriberConfig } from "@medusajs/framework"
import { AuthWorkflowEvents } from "@medusajs/framework/utils"
import type { RegistrationVerification } from "../lib/registration-email"
import { registrationEmailWorkflow } from "../workflows/registration-email"

export default async function registrationEmail({ event, container }: SubscriberArgs<RegistrationVerification>) {
  await registrationEmailWorkflow(container).run({ input: event.data })
}

export const config: SubscriberConfig = { event: AuthWorkflowEvents.VERIFICATION_REQUESTED }
