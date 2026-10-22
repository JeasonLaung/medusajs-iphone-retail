import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { MedusaError, Modules } from "@medusajs/framework/utils"
import { CustomerServiceBody } from "../api/admin/customer-service/validators"
import { customerServiceSettings } from "../lib/customer-service"

const saveCustomerServiceContacts = createStep("save-customer-service-contacts", async (input: CustomerServiceBody, { container }) => {
  const settings = CustomerServiceBody.parse(input)
  const { store } = await customerServiceSettings(container)
  if (!store) throw new MedusaError(MedusaError.Types.NOT_FOUND, "SERVICE_STORE_NOT_FOUND")
  const previous = store.metadata?.customer_service_contacts ?? null
  await container.resolve(Modules.STORE).updateStores(store.id, { metadata: { ...store.metadata, customer_service_contacts: settings.contacts } })
  return new StepResponse(settings, { id: store.id, previous })
}, async (snapshot, { container }) => {
  if (!snapshot) return
  const stores = container.resolve(Modules.STORE)
  const store = await stores.retrieveStore(snapshot.id)
  await stores.updateStores(store.id, { metadata: { ...store.metadata, customer_service_contacts: snapshot.previous } })
})

export const saveCustomerServiceWorkflow = createWorkflow("save-customer-service", function (input: CustomerServiceBody) {
  return new WorkflowResponse(saveCustomerServiceContacts(input))
})
