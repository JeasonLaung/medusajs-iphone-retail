import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { updateShipmentTrackingWorkflow } from "../../../../../../../workflows/update-shipment-tracking"
import type { UpdateShipmentTrackingBody } from "./validators"

export async function POST(req: AuthenticatedMedusaRequest<UpdateShipmentTrackingBody>, res: MedusaResponse) {
  const { result } = await updateShipmentTrackingWorkflow(req.scope).run({ input: {
    ...req.validatedBody,
    order_id: req.params.id,
    fulfillment_id: req.params.fulfillment_id,
    actor_id: req.auth_context.actor_id,
  } })
  res.json({ fulfillment: result })
}
