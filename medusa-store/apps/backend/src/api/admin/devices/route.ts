import type {
  AuthenticatedMedusaRequest,
  MedusaResponse,
} from "@medusajs/framework/http"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../../../modules/devices"
import DevicesModuleService from "../../../modules/devices/service"
import { deviceOperationWorkflow } from "../../../workflows/device-operations"
import { createDeviceSourceWorkflow } from "../../../workflows/device-source"
import type { DeviceCommandBody } from "./validators"

export async function GET(
  req: AuthenticatedMedusaRequest,
  res: MedusaResponse,
) {
  const service: DevicesModuleService = req.scope.resolve(DEVICES_MODULE)
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const q =
    typeof req.query.q === "string"
      ? req.query.q.trim().toUpperCase().slice(0, 100)
      : ""
  const order_id =
    typeof req.query.order_id === "string"
      ? req.query.order_id.slice(0, 100)
      : undefined
  const device_id =
    typeof req.query.device_id === "string"
      ? req.query.device_id.slice(0, 100)
      : undefined
  const offset = Math.max(0, Math.min(100000, Number(req.query.offset) || 0))
  let order: any = null
  if (order_id) {
    const { data } = await query.graph({
      entity: "order",
      fields: [
        "id",
        "display_id",
        "status",
        "currency_code",
        "items.id",
        "items.title",
        "items.variant_id",
        "items.variant_title",
        "items.detail.*",
        "fulfillments.id",
        "fulfillments.canceled_at",
        "fulfillments.shipped_at",
        "payment_collections.payments.id",
        "payment_collections.payments.captures.amount",
        "payment_collections.payments.refunds.amount",
        "payment_collections.payments.refunds.note",
      ],
      filters: { id: order_id },
    })
    order = data[0] || null
  }
  const assignments = await service.listDeviceAssignments(
    order_id ? { order_id } : device_id ? { device_id } : {},
    { take: 1000, order: { created_at: "DESC" } },
  )
  const searchFilter = {
    $or: [
      { serial_number: { $ilike: `%${q}%` } },
      { imei: { $ilike: `%${q}%` } },
      { id: { $ilike: `%${q.toLowerCase()}%` } },
    ],
  }
  const orderFilter = {
    $or: [
      { id: assignments.map((a) => a.device_id) },
      {
        variant_id:
          order?.items?.map((i: any) => i.variant_id).filter(Boolean) || [],
        state: "available",
      },
    ],
  }
  const filters = device_id
    ? { id: device_id }
    : order_id
      ? q
        ? { $and: [orderFilter, searchFilter] }
        : orderFilter
      : q
        ? searchFilter
        : {}
  const [devices, count] = await service.listAndCountDevices(filters, {
    take: 50,
    skip: offset,
    order: { created_at: "DESC" },
  })
  const sources = await service.listDeviceSources({}, { take: 500 })
  const supplies = await service.listDeviceSupplies({}, { take: 2000 })
  const managed_variants = await service.listDeviceVariants({}, { take: 2000 })
  const { data: variants } = await query.graph({
    entity: "product_variant",
    fields: [
      "id",
      "title",
      "sku",
      "product.title",
      "options.value",
      "inventory_items.inventory.location_levels.*",
    ],
    pagination: { take: 200 },
  })
  const assigned_devices = assignments.length
    ? await service.listDevices(
        { id: assignments.map((a) => a.device_id) },
        { take: 1000 },
      )
    : []
  const events = device_id
    ? await service.listDeviceEvents(
        { device_id },
        { take: 100, order: { created_at: "DESC" } },
      )
    : []
  res.json({
    devices,
    assigned_devices,
    count,
    offset,
    sources,
    supplies,
    managed_variants,
    variants,
    assignments,
    order,
    events,
  })
}

export async function POST(
  req: AuthenticatedMedusaRequest<DeviceCommandBody>,
  res: MedusaResponse,
) {
  const body = req.validatedBody
  if (body.action === "source") {
    const { result } = await createDeviceSourceWorkflow(req.scope).run({
      input: { name: body.name, type: body.type },
    })
    res.status(201).json({ source: result })
    return
  }
  const { result } = await deviceOperationWorkflow(req.scope).run({
    input: { ...body, actor_id: req.auth_context.actor_id },
  })
  res.json({ result })
}
