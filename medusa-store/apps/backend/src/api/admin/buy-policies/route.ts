import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { buyPolicySettings, type BuyPolicies } from "../../../lib/buy-policies"
import { saveBuyPoliciesWorkflow } from "../../../workflows/save-buy-policies"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const { policies } = await buyPolicySettings(req.scope)
  res.setHeader("Cache-Control", "no-store")
  res.json({ policies })
}

export async function POST(req: AuthenticatedMedusaRequest<BuyPolicies>, res: MedusaResponse) {
  const { result } = await saveBuyPoliciesWorkflow(req.scope).run({ input: req.validatedBody })
  res.json(result)
}
