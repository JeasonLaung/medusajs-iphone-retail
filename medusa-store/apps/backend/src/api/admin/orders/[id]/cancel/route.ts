import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import type { CancelOrderBody } from "./validators"
import { cancelOrderWithReasonWorkflow } from "../../../../../workflows/cancel-order-with-reason"

export async function POST(req: AuthenticatedMedusaRequest<CancelOrderBody>, res: MedusaResponse) {
  await cancelOrderWithReasonWorkflow(req.scope).run({
    input: { order_id: req.params.id, actor_id: req.auth_context.actor_id, reason: req.validatedBody.reason },
  })
  const { data } = await req.scope.resolve(ContainerRegistrationKeys.QUERY).graph({
    entity: "order", fields: req.queryConfig.fields, filters: { id: req.params.id },
  })
  res.status(200).json({ order: data[0] })
}
