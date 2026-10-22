import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { cancelOrderWorkflow } from "@medusajs/medusa/core-flows"
import { MedusaError, Modules } from "@medusajs/framework/utils"
import { CancelOrderBody } from "../api/admin/orders/[id]/cancel/validators"

type Input = { order_id: string; actor_id: string; reason: string }

const recordOrderCancellationReason = createStep("record-order-cancellation-reason", async (input: Input, { container }) => {
  const { reason } = CancelOrderBody.parse(input)
  const orders = container.resolve(Modules.ORDER)
  const order = await orders.retrieveOrder(input.order_id)
  if (order.status === "canceled" || order.canceled_at) {
    throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "Order has already been canceled")
  }
  const previous = order.metadata?.cancellation ?? null
  await orders.updateOrders([{
    id: order.id,
    metadata: {
      ...order.metadata,
      cancellation: { reason, actor_id: input.actor_id, recorded_at: new Date().toISOString() },
    },
  }])
  return new StepResponse({ order_id: order.id, canceled_by: input.actor_id }, { order_id: order.id, previous })
}, async (snapshot, { container }) => {
  if (!snapshot) return
  const orders = container.resolve(Modules.ORDER)
  const order = await orders.retrieveOrder(snapshot.order_id)
  // A completed cancellation remains auditable even if a later step fails.
  if (order.canceled_at) return
  await orders.updateOrders([{ id: order.id, metadata: { ...order.metadata, cancellation: snapshot.previous } }])
})

export const cancelOrderWithReasonWorkflow = createWorkflow("cancel-order-with-reason", function (input: Input) {
  const cancellation = recordOrderCancellationReason(input)
  cancelOrderWorkflow.runAsStep({ input: cancellation })
  return new WorkflowResponse(cancellation)
})
