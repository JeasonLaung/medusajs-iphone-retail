import type {
  AuthenticatedMedusaRequest,
  MedusaResponse,
} from "@medusajs/framework/http"
import {
  ContainerRegistrationKeys,
  MedusaError,
  Modules,
} from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../../../modules/devices"
import DevicesModuleService from "../../../modules/devices/service"
import { deviceOperationWorkflow } from "../../../workflows/device-operations"
import { createDeviceSourceWorkflow } from "../../../workflows/device-source"
import type { DeviceCommandBody } from "./validators"
import { requiresImei } from "../../../modules/devices/identity"
import { orderItemQuantities } from "../../../lib/order-item-quantities"
import {
  authorizeDeviceCommand,
  deviceAccess,
  supplierDenied,
} from "../../device-access"

export async function GET(
  req: AuthenticatedMedusaRequest,
  res: MedusaResponse,
) {
  const service: DevicesModuleService = req.scope.resolve(DEVICES_MODULE)
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const access = await deviceAccess(req.scope, req.auth_context.actor_id)
  const supplier = access.role === "supplier"
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
  const limit = Math.max(1, Math.min(100, Math.floor(Number(req.query.limit) || 50)))
  const lookupVariant = typeof req.query.lookup_variant_id === "string"
    ? req.query.lookup_variant_id.slice(0, 100) : undefined
  const lookupLocation = typeof req.query.lookup_location_id === "string"
    ? req.query.lookup_location_id.slice(0, 100) : undefined
  const category = typeof req.query.category === "string" ? req.query.category : undefined
  if (category && !["sale", "returned", "recycled"].includes(category)) throw new MedusaError(MedusaError.Types.INVALID_DATA, "Invalid device category")
  const lookupState = typeof req.query.lookup_state === "string" ? req.query.lookup_state : undefined
  if (lookupState && !["pending", "inspection", "available", "reserved", "packed", "shipped", "repair", "repair_ready", "retired", "supplier_returned"].includes(lookupState))
    throw new MedusaError(MedusaError.Types.INVALID_DATA, "Invalid device state")
  if (supplier && order_id && !access.order_ids.includes(order_id))
    throw supplierDenied()
  if (supplier && device_id) {
    const [device] = await service.listDevices({
      id: device_id,
      source_id: access.source_id!,
    })
    if (!device) throw supplierDenied()
  }
  let order: any = null
  if (order_id) {
    const { data } = await query.graph({
      entity: "order",
      fields: supplier
        ? [
            "id",
            "display_id",
            "status",
            "currency_code",
            "items.id",
            "items.title",
            "items.variant_id",
            "items.variant_title",
            "items.quantity",
            "items.detail.quantity",
            "items.detail.fulfilled_quantity",
            "fulfillments.id",
            "fulfillments.location_id",
            "fulfillments.shipped_at",
            "fulfillments.delivered_at",
            "fulfillments.canceled_at",
            "fulfillments.items.line_item_id",
            "fulfillments.items.quantity",
            "shipping_address.first_name",
            "shipping_address.last_name",
            "shipping_address.company",
            "shipping_address.address_1",
            "shipping_address.address_2",
            "shipping_address.city",
            "shipping_address.province",
            "shipping_address.postal_code",
            "shipping_address.country_code",
            "shipping_address.phone",
          ]
        : [
            "id",
            "display_id",
            "status",
            "currency_code",
            "items.id",
            "items.title",
            "items.variant_id",
            "items.variant_title",
            "items.quantity",
            "items.detail.quantity",
            "items.detail.fulfilled_quantity",
            "fulfillments.id",
            "fulfillments.location_id",
            "fulfillments.shipped_at",
            "fulfillments.delivered_at",
            "fulfillments.canceled_at",
            "fulfillments.items.line_item_id",
            "fulfillments.items.quantity",
            "payment_collections.payments.id",
            "payment_collections.payments.captures.amount",
            "payment_collections.payments.refunds.amount",
            "payment_collections.payments.refunds.note",
          ],
      filters: { id: order_id },
    })
    order = data[0] || null
    if (order) {
      const { data: orderVariants } = await query.graph({
        entity: "product_variant",
        fields: [
          "id",
          "product.title",
          "product.handle",
          "product.metadata",
          "product.type.value",
        ],
        filters: {
          id:
            order.items?.map((item: any) => item.variant_id).filter(Boolean) ||
            [],
        },
      })
      order.items = order.items?.map((item: any) => {
        const quantities = orderItemQuantities(item)
        return {
          ...item,
          quantity: quantities.quantity,
          detail: { ...item.detail, ...quantities },
          requires_imei: requiresImei(
            orderVariants.find((v) => v.id === item.variant_id)?.product,
          ),
        }
      })
    }
  }
  const assignments = await service.listDeviceAssignments(
    {
      ...(order_id ? { order_id } : device_id ? { device_id } : {}),
      ...(supplier
        ? {
            source_id: access.source_id!,
            order_id: order_id || access.order_ids,
          }
        : {}),
    },
    { take: 1000, order: { created_at: "DESC" } },
  )
  const searchFilter = {
    $or: [
      { serial_number: { $ilike: `%${q}%` } },
      { imei: { $ilike: `%${q}%` } },
      { imei2: { $ilike: `%${q}%` } },
      { eid: { $ilike: `%${q}%` } },
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
  // Order allocation pickers only list available devices for an item in this order.
  const availableVariant =
    typeof req.query.available_variant_id === "string"
      ? req.query.available_variant_id
      : undefined
  if (
    availableVariant &&
    !order?.items?.some((item: any) => item.variant_id === availableVariant)
  ) {
    throw new MedusaError(MedusaError.Types.INVALID_DATA, "商品不属于该订单")
  }
  const filters = availableVariant
    ? {
        $and: [
          { variant_id: availableVariant, state: "available" },
          ...(q ? [searchFilter] : []),
        ],
      }
    : device_id
      ? { id: device_id }
      : order_id
        ? q
          ? { $and: [orderFilter, searchFilter] }
          : orderFilter
        : q
          ? searchFilter
          : {}
  const [devices, count] = await service.listAndCountDevices(
    { $and: [filters, ...(lookupVariant ? [{ variant_id: lookupVariant }] : []),
      ...(lookupLocation ? [{ location_id: lookupLocation }] : []),
      ...(lookupState ? [{ state: lookupState }] : []),
      ...(category ? [{ category }] : []),
      ...(supplier ? [{ source_id: access.source_id! }] : [])] },
    {
      take: limit,
      skip: offset,
      order: { created_at: "DESC", id: "DESC" },
    },
  )
  // Counts describe this SKU's saleable stock across warehouses, independent of search/page.
  let allocationStock: { location_id: string; name: string; count: number }[] | undefined
  if (availableVariant) {
    const stock = await service.listDevices({
      variant_id: availableVariant,
      state: "available",
      category: "sale",
      ...(supplier ? { source_id: access.source_id! } : {}),
    }, { select: ["id", "location_id"], take: null })
    const fulfillmentLocations = order?.fulfillments?.map((f: any) => f.location_id).filter(Boolean) || []
    const locationIds = [...new Set<string>([...stock.map((d) => d.location_id), ...fulfillmentLocations])]
    const locations = locationIds.length
      ? await req.scope.resolve(Modules.STOCK_LOCATION).listStockLocations({ id: locationIds }, { take: null })
      : []
    allocationStock = locationIds.map((id) => ({
      location_id: id,
      name: locations.find((location) => location.id === id)?.name || id,
      count: stock.filter((device) => device.location_id === id).length,
    }))
  }
  const sources = await service.listDeviceSources(
    supplier ? { id: access.source_id! } : {},
    { take: 500 },
  )
  const supplies = await service.listDeviceSupplies(
    supplier ? { source_id: access.source_id! } : {},
    { take: 2000 },
  )
  const managed_variants = await service.listDeviceVariants({}, { take: 2000 })
  const { data: variants } = await query.graph({
    entity: "product_variant",
    fields: [
      "id",
      "title",
      "sku",
      "product.title",
      "product.handle",
      "product.metadata",
      "product.type.value",
      "options.value",
      "inventory_items.inventory.location_levels.*",
    ],
    pagination: { take: 200 },
  })
  const selectedVariantId = typeof req.query.selected_variant_id === "string"
    ? req.query.selected_variant_id.slice(0, 100) : undefined
  if (selectedVariantId && !variants.some((variant) => variant.id === selectedVariantId)) {
    const { data: selectedVariants } = await query.graph({
      entity: "product_variant",
      fields: ["id", "title", "sku", "product.title", "product.handle", "product.metadata", "product.type.value", "options.value", "inventory_items.inventory.location_levels.*"],
      filters: { id: selectedVariantId },
    })
    variants.push(...selectedVariants)
  }
  // Resolve every variant on this device page, including those beyond the picker preload.
  const missingVariantIds = [...new Set(devices.map((device) => device.variant_id))]
    .filter((id) => !variants.some((variant) => variant.id === id))
  if (missingVariantIds.length) {
    const { data: pageVariants } = await query.graph({
      entity: "product_variant",
      fields: ["id", "sku", "title", "product.title", "product.handle", "product.metadata", "product.type.value", "options.value", "inventory_items.inventory.location_levels.*"],
      filters: { id: missingVariantIds },
    })
    variants.push(...pageVariants)
  }
  for (const variant of variants) Object.assign(variant, { requires_imei: requiresImei(variant.product) })
  const assigned_devices = assignments.length
    ? await service.listDevices(
        {
          id: assignments.map((a) => a.device_id),
          ...(supplier ? { source_id: access.source_id! } : {}),
        },
        { take: 1000 },
      )
    : []
  const events = device_id
    ? await service.listDeviceEvents(
        { device_id },
        { take: 100, order: { created_at: "DESC" } },
      )
    : []
  const actorIds = [...new Set(events.map((event) => event.actor_id))]
  const actors = actorIds.length ? await req.scope.resolve(Modules.USER).listUsers({ id: actorIds }, { take: actorIds.length }) : []
  const eventActors = new Map(actors.map((actor) => [actor.id, [actor.first_name, actor.last_name].filter(Boolean).join(" ").trim() || (!supplier ? actor.email : null) || actor.id]))
  const history = devices.length ? await service.listDeviceAssignments({ device_id: devices.map((d) => d.id), status: ["reserved", "packed", "shipped", "returned"] }, { take: null }) : []
  const locked = new Set(history.map((a) => a.device_id))
  res.json({
    access,
    devices: devices.map((d) => ({ ...d, can_delete: !supplier && !locked.has(d.id) && !["reserved", "packed", "shipped", "repair_ready"].includes(d.state), can_retire: !supplier && !locked.has(d.id) && ["pending", "available", "inspection", "repair"].includes(d.state) })),
    assigned_devices,
    ...(allocationStock ? { allocation_stock: allocationStock } : {}),
    count,
    offset,
    limit,
    sources,
    supplies,
    managed_variants,
    variants: supplier
      ? variants.map((variant: any) => ({
          ...variant,
          inventory_items: variant.inventory_items?.map((item: any) => ({
            ...item,
            inventory: {
              ...item.inventory,
              location_levels:
                item.inventory?.location_levels?.filter(
                  (level: any) => level.location_id === sources[0]?.location_id,
                ) || [],
            },
          })),
        }))
      : variants,
    assignments,
    order,
    events: events.map((event) => ({ ...event, actor_name: eventActors.get(event.actor_id) || event.actor_id })),
  })
}

export async function POST(
  req: AuthenticatedMedusaRequest<DeviceCommandBody>,
  res: MedusaResponse,
) {
  const body = req.validatedBody
  await authorizeDeviceCommand(req.scope, req.auth_context.actor_id, body)
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
