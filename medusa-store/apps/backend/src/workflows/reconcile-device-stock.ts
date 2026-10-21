import { createStep, createWorkflow, StepResponse, WorkflowResponse, when } from "@medusajs/framework/workflows-sdk"
import { acquireLockStep, releaseLockStep, updateInventoryLevelsWorkflow } from "@medusajs/medusa/core-flows"

const prepareDeviceStockReconciliation = createStep("prepare-device-stock-reconciliation", async (_input: Record<string, never>, { container }) => {
  // Kept for existing callers: device counts no longer reconcile native stock.
  const updates: { id: string; inventory_item_id: string; location_id: string; stocked_quantity: number }[] = []
  return new StepResponse({ updates })
})

export const reconcileDeviceStockWorkflow = createWorkflow("reconcile-device-stock", function (input: Record<string, never>) {
  acquireLockStep({ key: "device-operations", timeout: 30, ttl: 180 })
  const plan = prepareDeviceStockReconciliation(input)
  when(plan, value => value.updates.length > 0).then(() => {
    updateInventoryLevelsWorkflow.runAsStep({ input: { updates: plan.updates } })
  })
  releaseLockStep({ key: "device-operations" })
  return new WorkflowResponse(plan)
})
