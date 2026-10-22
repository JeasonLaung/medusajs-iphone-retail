import type { IPaymentModuleService, PaymentSessionDTO } from "@medusajs/framework/types"
import { validatePaymentProviderInRegionStep } from "@medusajs/medusa/core-flows"
import { MedusaError, Modules } from "@medusajs/framework/utils"
import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"

export const mollieHostedProvider = "pp_mollie-hosted-checkout_mollie"

// Refresh provider state before the core workflow can create another charge or
// try to delete an already-paid session left behind by an earlier failed attempt.
export async function resumeMollieSession(payment: IPaymentModuleService, collectionId: string) {
  const collection = await payment.retrievePaymentCollection(collectionId, {
    relations: ["payment_sessions"],
  })
  const sessions = collection.payment_sessions || []
  const refreshed: PaymentSessionDTO[] = []
  for (const session of sessions) {
    if (session.provider_id !== mollieHostedProvider) continue
    refreshed.push(await payment.updatePaymentSession({
      id: session.id,
      amount: session.amount,
      currency_code: session.currency_code,
      data: session.data,
      context: session.context,
    }))
  }
  const paid = refreshed.filter((session) => session.status === "authorized")
  if (paid.length > 1)
    throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "MOLLIE_MULTIPLE_PAID_SESSIONS")
  const selected = paid[0] || refreshed.find((session) =>
    ["pending", "requires_more"].includes(session.status) &&
    Number(session.amount) === Number(collection.amount) &&
    session.currency_code === collection.currency_code
  )
  if (!selected) return null
  if (Number(selected.amount) !== Number(collection.amount) || selected.currency_code !== collection.currency_code)
    throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "MOLLIE_PAID_AMOUNT_MISMATCH")
  // Keep only one processable session: cart completion authorizes every active
  // session, including duplicates left by the failed core workflow.
  for (const session of sessions) {
    if (session.id !== selected.id) await payment.deletePaymentSession(session.id)
  }
  return selected.id
}

const resumeMollieSessionStep = createStep("resume-mollie-session", async (input: { collection_id: string }, { container }) => {
  const id = await resumeMollieSession(container.resolve(Modules.PAYMENT), input.collection_id)
  return new StepResponse(id)
})

export const resumeMollieSessionWorkflow = createWorkflow("resume-mollie-session", function (input: { collection_id: string }) {
  validatePaymentProviderInRegionStep({ payment_collection_id: input.collection_id, provider_id: mollieHostedProvider })
  return new WorkflowResponse(resumeMollieSessionStep(input))
})
