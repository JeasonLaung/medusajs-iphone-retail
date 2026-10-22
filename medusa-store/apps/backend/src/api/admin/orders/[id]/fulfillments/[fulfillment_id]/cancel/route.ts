import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { deviceOperationWorkflow } from "../../../../../../../workflows/device-operations"

export async function POST(
  req: AuthenticatedMedusaRequest<{ no_notification?: boolean }>,
  res: MedusaResponse,
) {
  await deviceOperationWorkflow(req.scope).run({
    input: {
      action: "cancel_fulfillment",
      fulfillment_id: req.params.fulfillment_id,
      actor_id: req.auth_context.actor_id,
      native_cancel: {
        ...req.validatedBody,
        order_id: req.params.id,
        canceled_by: req.auth_context.actor_id,
      },
    },
  })
  const { data } = await req.scope.resolve(ContainerRegistrationKeys.QUERY).graph({
    entity: "order",
    fields: req.queryConfig.fields,
    filters: { id: req.params.id },
  })
  res.status(200).json({ order: data[0] })
}
