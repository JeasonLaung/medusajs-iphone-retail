import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { createIntakeToken } from "../lib/mobile-intake-token"
import { intakeSecret, intakeSource, mobileIntakeAccess } from "../api/mobile-intake/access"
import { IntakeDevice, type IntakeDeviceBody } from "../api/admin/devices/validators"
import { deviceOperationWorkflow } from "./device-operations"

const createMobileIntakeLink = createStep("create-mobile-intake-link", async (input: { actor_id: string; source_id: string; category?: "sale" | "returned" | "recycled" }, { container }) => {
  await intakeSource(container, input.actor_id, input.source_id)
  return new StepResponse(createIntakeToken(input.actor_id, input.source_id, intakeSecret(container), Date.now(), input.category))
})

export const createMobileIntakeWorkflow = createWorkflow("create-mobile-intake", function (input: { actor_id: string; source_id: string; category?: "sale" | "returned" | "recycled" }) {
  return new WorkflowResponse(createMobileIntakeLink(input))
})

const prepareMobileIntake = createStep("prepare-mobile-intake", async (input: { token: string; device: Omit<IntakeDeviceBody, "source_id" | "category"> }, { container }) => {
  const { session } = await mobileIntakeAccess(container, input.token)
  const device = IntakeDevice.parse({ ...input.device, source_id: session.source_id, category: session.category })
  return new StepResponse({ action: "intake" as const, actor_id: session.actor_id, devices: [device] })
})

export const submitMobileIntakeWorkflow = createWorkflow("submit-mobile-intake", function (input: { token: string; device: Omit<IntakeDeviceBody, "source_id" | "category"> }) {
  const command = prepareMobileIntake(input)
  const result = deviceOperationWorkflow.runAsStep({ input: command })
  return new WorkflowResponse(result)
})
