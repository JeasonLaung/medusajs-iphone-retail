import type {
  MedusaRequest,
  MedusaResponse,
  MedusaNextFunction,
} from "@medusajs/framework/http"
import {
  ContainerRegistrationKeys,
  MedusaError,
} from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"

export async function guardDeviceOrder(
  req: MedusaRequest,
  _res: MedusaResponse,
  next: MedusaNextFunction,
) {
  // Shipment validation and device synchronization are handled by our shipment route.
  if (req.path.endsWith("/shipments")) return next()
  const service: DevicesModuleService = req.scope.resolve(DEVICES_MODULE)
  const order_id = req.params.id
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const { data } = await query.graph({
    entity: "order",
    fields: ["id", "items.variant_id"],
    filters: { id: order_id },
  })
  const variants = await service.listDeviceVariants({
    variant_id:
      data[0]?.items
        ?.map((i) => i?.variant_id)
        .filter((id): id is string => Boolean(id)) || [],
  })
  const active = await service.listDeviceAssignments({
    order_id,
    status: ["reserved", "packed", "shipped"],
  })
  if (!variants.length && !active.length) return next()
  if (req.path.endsWith("/cancel") && !req.path.includes("/fulfillments/")) {
    if (!active.length) return next()
  }
  throw new MedusaError(
    MedusaError.Types.NOT_ALLOWED,
    "该订单包含序列号设备，请在订单详情的设备与履约区域完成配货、履约、发货或解除配货",
  )
}

export async function guardDeviceInventory(
  req: MedusaRequest,
  _res: MedusaResponse,
  next: MedusaNextFunction,
) {
  // Device tracking does not own or restrict native quantity inventory.
  next()
}

export async function guardDeviceReturn(
  req: MedusaRequest,
  _res: MedusaResponse,
  next: MedusaNextFunction,
) {
  const service: DevicesModuleService = req.scope.resolve(DEVICES_MODULE)
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  let order_id = (req.body as { order_id?: string })?.order_id
  if (!order_id && req.params.id) {
    const { data } = await query.graph({
      entity: req.path.includes("/exchanges/") ? "exchange" : "return",
      fields: ["order_id"],
      filters: { id: req.params.id },
    })
    order_id = data[0]?.order_id
  }
  if (order_id && (await service.listDeviceAssignments({ order_id })).length)
    throw new MedusaError(
      MedusaError.Types.NOT_ALLOWED,
      "序列号设备退回必须在扫码工作台核对原机并进入待检测，请从订单售后申请处理",
    )
  next()
}
