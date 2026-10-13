import type {
  AuthenticatedMedusaRequest,
  MedusaResponse,
} from "@medusajs/framework/http"
import { deviceRefundWorkflow } from "../../../../workflows/device-refund"
import type { DeviceRefundBody } from "../validators"

export async function POST(
  req: AuthenticatedMedusaRequest<DeviceRefundBody>,
  res: MedusaResponse,
) {
  const { result } = await deviceRefundWorkflow(req.scope).run({
    input: { ...req.validatedBody, actor_id: req.auth_context.actor_id },
  })
  res.json({ result })
}
