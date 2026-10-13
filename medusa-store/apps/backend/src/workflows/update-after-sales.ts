import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { MedusaError, ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { acquireLockStep, releaseLockStep } from "@medusajs/medusa/core-flows"
import { AFTER_SALES_MODULE } from "../modules/after-sales"
import AfterSalesModuleService from "../modules/after-sales/service"
import { serializeAfterSales } from "../modules/after-sales/serialize"
import type { UpdateAfterSalesBody } from "../api/admin/orders/[id]/after-sales/validators"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"

type Input = UpdateAfterSalesBody & { order_id: string }

const updateRequestStep = createStep("update-request", async (input: Input, { container }) => {
  const service: AfterSalesModuleService = container.resolve(AFTER_SALES_MODULE)
  const existing = await service.retrieveAfterSalesRequest(input.request_id)
  if (existing.order_id !== input.order_id) {
    throw new MedusaError(MedusaError.Types.NOT_FOUND, "Request not found")
  }
  if (existing.status === "completed" || existing.status === "rejected") {
    throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "This request is already closed")
  }
  const devices: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const received = await devices.listDeviceAssignments({ after_sales_request_id: existing.id })
  if (input.status === "rejected" && received.length) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "已有设备收回记录，不能直接拒绝申请")
  if (input.status === "completed") {
    const lines = (existing.items as { lines: { device_ids?: string[] }[] }).lines
    const ids = lines.flatMap((l) => l.device_ids || [])
    for (const id of ids) {
      const original = received.find((a) => a.device_id === id && !a.replacement_of)
      const device = await devices.retrieveDevice(id)
      if (!original || ["inspection", "repair", "repair_ready"].includes(device.state)) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "请先收回申请中的设备并完成检测 / 维修处理")
      if (existing.type === "repair" && original.status !== "shipped") throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "维修原机尚未返还客户")
      if (existing.type === "exchange" && !received.some((a) => a.replacement_of === original.id && a.status === "shipped")) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "替换设备尚未发货")
    }
    if (ids.length && existing.type === "return") {
      const query = container.resolve(ContainerRegistrationKeys.QUERY)
      const { data } = await query.graph({ entity: "order", fields: ["payment_collections.payments.refunds.note", "payment_collections.payments.refunds.amount"], filters: { id: existing.order_id } })
      const refunded = data[0]?.payment_collections?.flatMap((c) => c?.payments || []).flatMap((p) => p?.refunds || []).some((r) => r?.note === `after-sales:${existing.id}` && Number(r.amount) > 0)
      if (!refunded) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "退货退款尚未执行，请在扫码工作台核对金额并执行退款后再完成申请")
    }
  }
  const request = await service.updateAfterSalesRequests({ id: existing.id, status: input.status })
  return new StepResponse(serializeAfterSales(request), { id: existing.id, status: existing.status })
}, async (previous: { id: string; status: "pending" | "processing" } | undefined, { container }) => {
  if (previous) {
    const service: AfterSalesModuleService = container.resolve(AFTER_SALES_MODULE)
    await service.updateAfterSalesRequests(previous)
  }
})

export const updateAfterSalesWorkflow = createWorkflow("update-after-sales", (input: Input) => {
  acquireLockStep({ key: "device-operations", timeout: 30, ttl: 180 }).config({ name: "lock-device-after-sales" })
  acquireLockStep({ key: input.order_id, timeout: 10, ttl: 60 })
  const request = updateRequestStep(input)
  releaseLockStep({ key: input.order_id })
  releaseLockStep({ key: "device-operations" }).config({ name: "unlock-device-after-sales" })
  return new WorkflowResponse(request)
})
