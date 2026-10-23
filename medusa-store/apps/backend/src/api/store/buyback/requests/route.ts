import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { BUYBACK_MODULE } from "../../../../modules/buyback"
import type BuybackModuleService from "../../../../modules/buyback/service"
import { summarizeBuybackRequests } from "../../../../lib/buyback-summary"
import { serializeBuyback } from "../../../../lib/buyback"
import { createBuybackWorkflow } from "../../../../workflows/buyback"
import type { CreateBuyback } from "../../../buyback-validators"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const service = req.scope.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const offset = Math.max(0, Math.min(100000, Math.floor(Number(req.query.offset) || 0)))
  const [requests, count] = await service.listAndCountBuybackRequests({ customer_id: req.auth_context.actor_id }, { take: 20, skip: offset, order: { created_at: "DESC" } })
  const allRequests = await service.listBuybackRequests({ customer_id: req.auth_context.actor_id }, { take: null, select: ["status", "items", "currency_code", "final_amount"] })
  res.json({ summary: summarizeBuybackRequests(allRequests), requests: await Promise.all(requests.map((request) => serializeBuyback(req.scope, request))), count, offset, limit: 20 })
}
export async function POST(req: AuthenticatedMedusaRequest<CreateBuyback>, res: MedusaResponse) {
  const { result } = await createBuybackWorkflow(req.scope).run({ input: { ...req.validatedBody, customer_id: req.auth_context.actor_id } })
  res.status(201).json({ request: result })
}
