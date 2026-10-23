import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { BUYBACK_MODULE } from "../../../../modules/buyback"
import type BuybackModuleService from "../../../../modules/buyback/service"
import { adminBuybackAccess, serializeBuyback } from "../../../../lib/buyback"
import { updateBuybackWorkflow } from "../../../../workflows/buyback"
import type { BuybackAdminCommand } from "../../../buyback-validators"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  await adminBuybackAccess(req.scope, req.auth_context.actor_id)
  const request = await req.scope.resolve<BuybackModuleService>(BUYBACK_MODULE).retrieveBuybackRequest(req.params.id)
  res.json({ request: await serializeBuyback(req.scope, request) })
}
export async function POST(req: AuthenticatedMedusaRequest<BuybackAdminCommand>, res: MedusaResponse) {
  const { result } = await updateBuybackWorkflow(req.scope).run({ input: { request_id: req.params.id, actor_id: req.auth_context.actor_id, command: req.validatedBody } })
  res.json({ request: result })
}
