import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { MedusaError, Modules } from "@medusajs/framework/utils"
import { BuyPoliciesBody, buyPolicySettings, type BuyPolicies } from "../lib/buy-policies"

const saveBuyPoliciesStep = createStep("save-buy-policies", async (input: BuyPolicies, { container }) => {
  const policies = BuyPoliciesBody.parse(input)
  const { store } = await buyPolicySettings(container)
  if (!store) throw new MedusaError(MedusaError.Types.NOT_FOUND, "BUY_POLICY_STORE_NOT_FOUND")
  const previous = store.metadata?.buy_policies ?? null
  await container.resolve(Modules.STORE).updateStores(store.id, {
    metadata: { ...store.metadata, buy_policies: policies },
  })
  return new StepResponse({ policies }, { id: store.id, previous })
}, async (snapshot, { container }) => {
  if (!snapshot) return
  const stores = container.resolve(Modules.STORE)
  const store = await stores.retrieveStore(snapshot.id)
  await stores.updateStores(store.id, { metadata: { ...store.metadata, buy_policies: snapshot.previous } })
})

export const saveBuyPoliciesWorkflow = createWorkflow("save-buy-policies", function (input: BuyPolicies) {
  return new WorkflowResponse(saveBuyPoliciesStep(input))
})
