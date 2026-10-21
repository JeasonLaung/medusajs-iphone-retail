import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import type { AdditionalData, HttpTypes } from "@medusajs/framework/types"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { createOrderShipmentWorkflow } from "@medusajs/medusa/core-flows"
import { DEVICES_MODULE } from "../../../../../../../modules/devices"
import DevicesModuleService from "../../../../../../../modules/devices/service"
import { deviceOperationWorkflow } from "../../../../../../../workflows/device-operations"

export async function POST(
  req: AuthenticatedMedusaRequest<HttpTypes.AdminCreateOrderShipment & AdditionalData>,
  res: MedusaResponse,
) {
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const service: DevicesModuleService = req.scope.resolve(DEVICES_MODULE)
  const order_id = req.params.id
  const fulfillment_id = req.params.fulfillment_id
  const { data } = await query.graph({
    entity: "order",
    fields: ["id", "items.id", "items.variant_id", "fulfillments.id", "fulfillments.items.line_item_id"],
    filters: { id: order_id },
  })
  const parcel = data[0]?.fulfillments?.find((f) => f?.id === fulfillment_id)
  const itemIds = new Set(parcel?.items?.map((item) => item?.line_item_id))
  const variantIds = data[0]?.items?.filter((item) => item && itemIds.has(item.id)).map((item) => item!.variant_id).filter((id): id is string => Boolean(id)) || []
  const managed = variantIds.length ? await service.listDeviceVariants({ variant_id: variantIds }) : []
  const assignments = await service.listDeviceAssignments({ order_id, fulfillment_id })
  const created_by = req.secret_key_context?.created_by ?? req.auth_context.actor_id
  const shipment = { ...req.validatedBody, order_id, fulfillment_id, labels: req.validatedBody.labels ?? [], created_by }

  if (managed.length || assignments.length) {
    await deviceOperationWorkflow(req.scope).run({
      input: {
        action: "ship",
        fulfillment_id,
        tracking_number: shipment.labels[0]?.tracking_number || "",
        actor_id: created_by,
        native_shipment: shipment,
      },
    })
  } else {
    await createOrderShipmentWorkflow(req.scope).run({ input: shipment })
  }
  const { data: orders } = await query.graph({
    entity: "order",
    fields: req.queryConfig.fields,
    filters: { id: order_id },
  })
  res.status(200).json({ order: orders[0] })
}
