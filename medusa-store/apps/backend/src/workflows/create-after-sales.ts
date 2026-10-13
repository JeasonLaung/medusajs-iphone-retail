import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"
import { acquireLockStep, releaseLockStep } from "@medusajs/medusa/core-flows"
import { AFTER_SALES_MODULE } from "../modules/after-sales"
import AfterSalesModuleService from "../modules/after-sales/service"
import { serializeAfterSales } from "../modules/after-sales/serialize"
import type { CreateAfterSalesBody } from "../api/store/orders/[id]/after-sales/validators"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"

type Input = CreateAfterSalesBody & { order_id: string; customer_id: string }

const createRequestStep = createStep("create-request", async (input: Input, { container }) => {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data: orders } = await query.graph({
    entity: "order",
    fields: ["id", "customer_id", "status", "items.id", "items.variant_id", "items.detail.quantity", "items.title"],
    filters: { id: input.order_id },
  })
  const order = orders[0]
  if (!order || order.customer_id !== input.customer_id) {
    throw new MedusaError(MedusaError.Types.NOT_FOUND, "Order not found")
  }
  if (order.status === "canceled" || order.status === "draft") {
    throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "This order cannot receive an after-sales request")
  }
  const ids = new Set<string>()
  const devices: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const assignments = await devices.listDeviceAssignments({ order_id: input.order_id, status: "shipped" })
  const managed = await devices.listDeviceVariants({})
  const items = input.items.map((requested) => {
    const item = order.items?.find((item) => item?.id === requested.item_id)
    const available = Number(item?.detail?.quantity)
    if (!item || !Number.isFinite(available) || ids.has(requested.item_id) || requested.quantity > available) {
      throw new MedusaError(MedusaError.Types.INVALID_DATA, "Choose valid order items and quantities")
    }
    ids.add(requested.item_id)
    if (managed.some((v) => v.variant_id === item.variant_id)) {
      const deviceIds = requested.device_ids || []
      if (deviceIds.length !== requested.quantity || new Set(deviceIds).size !== deviceIds.length || !deviceIds.every((id) => assignments.some((a) => a.device_id === id && a.item_id === item.id))) {
        throw new MedusaError(MedusaError.Types.INVALID_DATA, "Select the shipped devices for this order item")
      }
    } else if (requested.device_ids?.length) {
      throw new MedusaError(MedusaError.Types.INVALID_DATA, "This order item has no registered devices")
    }
    return { ...requested, title: item.title }
  })
  const service: AfterSalesModuleService = container.resolve(AFTER_SALES_MODULE)
  const existing = await service.listAfterSalesRequests({
    order_id: input.order_id,
    status: ["pending", "processing"],
  })
  if (existing.length) {
    throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "An after-sales request is already open for this order")
  }
  const request = await service.createAfterSalesRequests({
    order_id: input.order_id,
    customer_id: input.customer_id,
    type: input.type,
    reason: input.reason,
    items: { lines: items },
  })
  return new StepResponse(serializeAfterSales(request), request.id)
}, async (id: string | undefined, { container }) => {
  if (id) {
    const service: AfterSalesModuleService = container.resolve(AFTER_SALES_MODULE)
    await service.deleteAfterSalesRequests(id)
  }
})

export const createAfterSalesWorkflow = createWorkflow("create-after-sales", (input: Input) => {
  acquireLockStep({ key: input.order_id, timeout: 10, ttl: 60 })
  const request = createRequestStep(input)
  releaseLockStep({ key: input.order_id })
  return new WorkflowResponse(request)
})
