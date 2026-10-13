import {
  createStep,
  createWorkflow,
  StepResponse,
  WorkflowResponse,
  when,
  transform,
} from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import {
  acquireLockStep,
  releaseLockStep,
  refundPaymentWorkflow,
} from "@medusajs/medusa/core-flows"
import { AFTER_SALES_MODULE } from "../modules/after-sales"
import AfterSalesModuleService from "../modules/after-sales/service"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"
import { requireDevice } from "../modules/devices/policy"

type Input = {
  request_id: string
  payment_id: string
  amount: number
  actor_id: string
}
const prepareDeviceRefund = createStep(
  "prepare-device-refund",
  async (input: Input, { container }) => {
    const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
    const request = await container
      .resolve<AfterSalesModuleService>(AFTER_SALES_MODULE)
      .retrieveAfterSalesRequest(input.request_id)
    requireDevice(
      request.type === "return" && request.status === "processing",
      "只能为处理中退货退款申请执行退款",
    )
    const lines = (request.items as { lines: { device_ids?: string[] }[] })
      .lines
    const ids = lines.flatMap((line) => line.device_ids || [])
    requireDevice(ids.length > 0, "该申请没有序列号设备，请使用原生订单退款")
    const received = await service.listDeviceAssignments({
      after_sales_request_id: request.id,
      status: "returned",
    })
    requireDevice(
      ids.every((id) => received.some((a) => a.device_id === id)),
      "请先核对收回本次申请的全部机器",
    )
    const query = container.resolve(ContainerRegistrationKeys.QUERY)
    const { data } = await query.graph({
      entity: "order",
      fields: [
        "id",
        "payment_collections.payments.id",
        "payment_collections.payments.captures.amount",
        "payment_collections.payments.refunds.amount",
        "payment_collections.payments.refunds.note",
      ],
      filters: { id: request.order_id },
    })
    const payments =
      data[0]?.payment_collections?.flatMap((c) => c?.payments || []) || []
    const note = `after-sales:${request.id}`
    const existing = payments
      .flatMap((p) => p?.refunds || [])
      .find((r) => r?.note === note)
    if (existing)
      return new StepResponse({
        existing: true,
        note,
        amount: Number(existing.amount),
        payment_id: input.payment_id,
      })
    const payment = payments.find((p) => p?.id === input.payment_id)
    requireDevice(payment, "支付记录不属于本订单")
    const captured =
      payment.captures?.reduce((sum, c) => sum + Number(c?.amount || 0), 0) || 0
    const refunded =
      payment.refunds?.reduce((sum, r) => sum + Number(r?.amount || 0), 0) || 0
    requireDevice(
      input.amount > 0 && input.amount <= captured - refunded + 0.000001,
      "退款金额必须大于 0 且不超过剩余实收金额",
    )
    return new StepResponse({
      existing: false,
      note,
      amount: input.amount,
      payment_id: input.payment_id,
    })
  },
)

export const deviceRefundWorkflow = createWorkflow(
  "device-refund",
  (input: Input) => {
    acquireLockStep({ key: "device-operations", timeout: 30, ttl: 180 })
    const prepared = prepareDeviceRefund(input)
    when("execute-device-refund", prepared, (p) => !p.existing).then(() => {
      refundPaymentWorkflow.runAsStep({
        input: transform({ prepared, input }, ({ prepared, input }) => ({
          payment_id: prepared.payment_id,
          amount: prepared.amount,
          note: prepared.note,
          created_by: input.actor_id,
        })),
      })
    })
    releaseLockStep({ key: "device-operations" })
    return new WorkflowResponse(prepared)
  },
)
