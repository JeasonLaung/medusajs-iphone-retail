import {
  createWorkflow,
  WorkflowResponse,
} from "@medusajs/framework/workflows-sdk"
import { acquireLockStep, releaseLockStep } from "@medusajs/medusa/core-flows"
import { processMailQueueStep } from "./steps/process-mail-queue"

export const processMailQueueWorkflow = createWorkflow(
  "process-mail-queue",
  function (input: Record<string, never>) {
    acquireLockStep({ key: "mail-operations", timeout: 5, ttl: 180 })
    const result = processMailQueueStep(input)
    releaseLockStep({ key: "mail-operations" })
    return new WorkflowResponse(result)
  }
)
