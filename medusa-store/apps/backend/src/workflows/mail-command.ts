import {
  createWorkflow,
  WorkflowResponse,
} from "@medusajs/framework/workflows-sdk"
import { acquireLockStep, releaseLockStep } from "@medusajs/medusa/core-flows"
import { mailCommandStep } from "./steps/mail-command"
import type { MailCommandBody } from "../api/admin/mail/validators"

export const mailCommandWorkflow = createWorkflow(
  "mail-command",
  function (input: MailCommandBody & { actor_id: string }) {
    acquireLockStep({ key: "mail-operations", timeout: 30, ttl: 180 })
    const result = mailCommandStep(input)
    releaseLockStep({ key: "mail-operations" })
    return new WorkflowResponse(result)
  }
)
