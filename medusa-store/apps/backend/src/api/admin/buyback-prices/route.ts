import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { BUYBACK_MODULE } from "../../../modules/buyback"
import type BuybackModuleService from "../../../modules/buyback/service"
import { adminBuybackAccess } from "../../../lib/buyback"
import { setBuybackPriceWorkflow } from "../../../workflows/buyback"
import type { BuybackReferencePrice } from "../../buyback-validators"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  await adminBuybackAccess(req.scope, req.auth_context.actor_id)
  const productId = typeof req.query.product_id === "string" ? req.query.product_id : ""
  const service = req.scope.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const prices = productId ? await service.listBuybackPrices({ product_id: productId }, { take: 100 }) : []
  res.json({ prices })
}
export async function POST(req: AuthenticatedMedusaRequest<BuybackReferencePrice>, res: MedusaResponse) {
  const { result } = await setBuybackPriceWorkflow(req.scope).run({ input: { ...req.validatedBody, actor_id: req.auth_context.actor_id } })
  res.json({ price: result })
}
