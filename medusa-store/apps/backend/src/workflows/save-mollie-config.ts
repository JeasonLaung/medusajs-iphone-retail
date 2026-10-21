import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { MOLLIE_SETTINGS_MODULE } from "../modules/mollie-settings"

export type MollieConfigInput = { environment: "test" | "production"; redirect_url: string; enabled: boolean; api_key?: string }

const saveMollieConfigStep = createStep("save-mollie-config", async (input: MollieConfigInput, { container }) => {
  const service = container.resolve(MOLLIE_SETTINGS_MODULE) as any
  const [configs] = await service.listAndCountMollieConfigs({}, { take: 1 })
  const saved = configs[0]
    ? await service.updateMollieConfigs({ id: configs[0].id, ...input })
    : await service.createMollieConfigs(input)
  return new StepResponse(saved)
})

export const saveMollieConfigWorkflow = createWorkflow("save-mollie-config", function (input: MollieConfigInput) {
  const result = saveMollieConfigStep(input)
  return new WorkflowResponse(result)
})
