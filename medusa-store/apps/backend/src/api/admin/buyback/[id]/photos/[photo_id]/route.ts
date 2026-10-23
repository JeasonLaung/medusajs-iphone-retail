import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { adminBuybackAccess } from "../../../../../../lib/buyback"
import { readBuybackPhoto } from "../../../../../buyback-photo"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  await adminBuybackAccess(req.scope, req.auth_context.actor_id)
  res.setHeader("Cache-Control", "private, no-store")
  res.json(await readBuybackPhoto(req.scope, req.params.id, req.params.photo_id))
}
