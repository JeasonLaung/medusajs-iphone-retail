import {
  createStep,
  createWorkflow,
  StepResponse,
  WorkflowResponse,
} from "@medusajs/framework/workflows-sdk"
import { MedusaError, Modules } from "@medusajs/framework/utils"
import {
  TrustpilotBody,
  trustpilotSettings,
  type TrustpilotSettings,
} from "../lib/trustpilot"

const saveTrustpilotStep = createStep(
  "save-trustpilot",
  async (input: TrustpilotSettings, { container }) => {
    const settings = TrustpilotBody.parse(input)
    const { store } = await trustpilotSettings(container)
    if (!store)
      throw new MedusaError(
        MedusaError.Types.NOT_FOUND,
        "TRUSTPILOT_STORE_NOT_FOUND",
      )
    const previous = store.metadata?.trustpilot ?? null
    await container
      .resolve(Modules.STORE)
      .updateStores(store.id, {
        metadata: { ...store.metadata, trustpilot: settings },
      })
    return new StepResponse({ settings }, { id: store.id, previous })
  },
  async (snapshot, { container }) => {
    if (!snapshot) return
    const stores = container.resolve(Modules.STORE)
    const store = await stores.retrieveStore(snapshot.id)
    await stores.updateStores(store.id, {
      metadata: { ...store.metadata, trustpilot: snapshot.previous },
    })
  },
)

export const saveTrustpilotWorkflow = createWorkflow(
  "save-trustpilot",
  function (input: TrustpilotSettings) {
    return new WorkflowResponse(saveTrustpilotStep(input))
  },
)
