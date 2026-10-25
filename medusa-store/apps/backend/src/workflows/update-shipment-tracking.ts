import type { UpdateFulfillmentDTO } from "@medusajs/framework/types"
import { createStep, createWorkflow, StepResponse, WorkflowResponse, transform } from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"
import { acquireLockStep, releaseLockStep, updateFulfillmentStep } from "@medusajs/medusa/core-flows"
import { deviceAccess, supplierDenied } from "../api/device-access"
import { warehouseAccess, warehouseDenied } from "../lib/warehouse-access"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"
import type { UpdateShipmentTrackingBody } from "../api/admin/orders/[id]/fulfillments/[fulfillment_id]/tracking/validators"

type Input = UpdateShipmentTrackingBody & { order_id: string; fulfillment_id: string; actor_id: string }

export const shipmentTrackingContext = createStep("shipment-tracking-context", async (input: Input, { container }) => {
  const access = await deviceAccess(container, input.actor_id)
  if (access.role === "supplier") throw supplierDenied()
  if ((await warehouseAccess(container, input.actor_id)).warehouse) throw warehouseDenied()
  const { data: orders } = await container.resolve(ContainerRegistrationKeys.QUERY).graph({
    entity: "order",
    fields: ["id", "fulfillments.id", "fulfillments.shipped_at", "fulfillments.canceled_at", "fulfillments.labels.*"],
    filters: { id: input.order_id },
  })
  const fulfillment = orders[0]?.fulfillments?.find((entry) => entry?.id === input.fulfillment_id)
  if (!fulfillment || !fulfillment.shipped_at || fulfillment.canceled_at) {
    throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "SHIPMENT_TRACKING_NOT_SHIPPED")
  }
  const existing = fulfillment.labels || []
  const ids = input.labels.flatMap((label) => label.id ? [label.id] : [])
  if (new Set(ids).size !== ids.length || ids.some((id) => !existing.some((label) => label?.id === id)) || existing.some((label) => label && !ids.includes(label.id))) {
    throw new MedusaError(MedusaError.Types.INVALID_DATA, "SHIPMENT_TRACKING_STALE_LABELS")
  }
  const labels = input.labels.map((label) => ({
    ...label,
    tracking_url: label.tracking_url || "#",
    label_url: existing.find((entry) => entry?.id === label.id)?.label_url || "#",
  }))
  return new StepResponse({ id: input.fulfillment_id, labels } satisfies UpdateFulfillmentDTO & { id: string })
})

export const syncShipmentDeviceTracking = createStep("sync-shipment-device-tracking", async (input: Input, { container }) => {
  const service = container.resolve<DevicesModuleService>(DEVICES_MODULE)
  const assignments = await service.listDeviceAssignments({ order_id: input.order_id, fulfillment_id: input.fulfillment_id })
  const previous = assignments.map((assignment) => ({ id: assignment.id, tracking_number: assignment.tracking_number }))
  if (assignments.length) await service.updateDeviceAssignments(assignments.map((assignment) => ({ id: assignment.id, tracking_number: input.labels[0].tracking_number })))
  return new StepResponse(null, previous)
}, async (previous, { container }) => {
  if (previous?.length) await container.resolve<DevicesModuleService>(DEVICES_MODULE).updateDeviceAssignments(previous)
})

export const updateShipmentTrackingWorkflow = createWorkflow("update-shipment-tracking", function (input: Input) {
  acquireLockStep({ key: "device-operations", timeout: 30, ttl: 180 })
  const context = shipmentTrackingContext(input)
  const fulfillment = updateFulfillmentStep(context)
  // Make synchronization depend on the native update, so compensation restores labels on failure.
  const syncInput = transform({ input, fulfillment }, ({ input }) => input)
  syncShipmentDeviceTracking(syncInput)
  releaseLockStep({ key: "device-operations" })
  return new WorkflowResponse(fulfillment)
})
