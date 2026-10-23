import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { customerBuyback, serializeBuyback } from "../../../../../lib/buyback"
import { updateBuybackWorkflow } from "../../../../../workflows/buyback"
import type { BuybackCustomerCommand } from "../../../../buyback-validators"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const request = await customerBuyback(req.scope, req.params.id, req.auth_context.actor_id)
  res.json({ request: await serializeBuyback(req.scope, request) })
}
export async function POST(req: AuthenticatedMedusaRequest<BuybackCustomerCommand>, res: MedusaResponse) {
  const { result } = await updateBuybackWorkflow(req.scope).run({ input: { request_id: req.params.id, customer_id: req.auth_context.actor_id, command: req.validatedBody } })
  res.json({ request: result })
}
