import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { adminBuybackAccess } from "../../../lib/buyback"
import { buybackCatalog } from "../../../lib/buyback-catalog"
import type { BuybackCatalogCommand } from "../../buyback-pricing-validators"
import { updateBuybackCatalogWorkflow } from "../../../workflows/buyback-catalog"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  await adminBuybackAccess(req.scope, req.auth_context.actor_id)
  res.json(await buybackCatalog(req.scope))
}
export async function POST(req: AuthenticatedMedusaRequest<BuybackCatalogCommand>, res: MedusaResponse) {
  const { result } = await updateBuybackCatalogWorkflow(req.scope).run({ input: { actor_id: req.auth_context.actor_id, command: req.validatedBody } })
  res.json(result)
}
