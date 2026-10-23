import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { seoCommand, type SeoCommand } from "../lib/seo/operations"
const seoCommandStep = createStep("seo-command", async (input: SeoCommand, { container }) => new StepResponse(await seoCommand(container, input)))
export const seoCommandWorkflow = createWorkflow("seo-command", function (input: SeoCommand) {
  return new WorkflowResponse(seoCommandStep(input))
})
