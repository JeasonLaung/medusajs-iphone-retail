import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { buyPolicySettings } from "../../../lib/buy-policies"

export async function GET(req: MedusaRequest, res: MedusaResponse) {
  const { policies } = await buyPolicySettings(req.scope)
  res.setHeader("Cache-Control", "no-store")
  res.json({ policies })
}
