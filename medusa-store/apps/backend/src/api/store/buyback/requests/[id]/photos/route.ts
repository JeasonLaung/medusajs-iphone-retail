import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import type { BuybackPhotoUpload } from "../../../../../buyback-validators"
import { uploadBuybackPhotoWorkflow } from "../../../../../../workflows/buyback"

export async function POST(req: AuthenticatedMedusaRequest<BuybackPhotoUpload>, res: MedusaResponse) {
  const { result } = await uploadBuybackPhotoWorkflow(req.scope).run({ input: { ...req.validatedBody, request_id: req.params.id, customer_id: req.auth_context.actor_id } })
  res.status(201).json({ photo: result })
}
