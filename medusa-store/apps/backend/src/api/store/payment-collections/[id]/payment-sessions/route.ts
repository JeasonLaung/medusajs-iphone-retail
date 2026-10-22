import type { MedusaStoreRequest, MedusaResponse } from "@medusajs/framework/http"
import { refetchEntity } from "@medusajs/framework/http"
import { createPaymentSessionsWorkflow } from "@medusajs/medusa/core-flows"
import { mollieHostedProvider, resumeMollieSessionWorkflow } from "../../../../../workflows/resume-mollie-session"

export async function POST(req: MedusaStoreRequest, res: MedusaResponse) {
  const { provider_id, data } = req.validatedBody as { provider_id: string; data?: Record<string, unknown> }
  const collectionId = req.params.id
  const existing = provider_id === mollieHostedProvider
    ? (await resumeMollieSessionWorkflow(req.scope).run({ input: { collection_id: collectionId } })).result
    : null
  if (!existing) {
    await createPaymentSessionsWorkflow(req.scope).run({ input: {
      payment_collection_id: collectionId,
      provider_id,
      customer_id: req.auth_context?.actor_id,
      data,
    } })
  }
  const collection = await refetchEntity({
    entity: "payment_collection",
    idOrFilter: collectionId,
    scope: req.scope,
    fields: req.queryConfig.fields,
  })
  res.status(200).json({ payment_collection: collection })
}
