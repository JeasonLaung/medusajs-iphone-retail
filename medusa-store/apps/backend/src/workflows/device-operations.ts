import {
  createStep,
  createWorkflow,
  StepResponse,
  WorkflowResponse,
  transform,
  when,
} from "@medusajs/framework/workflows-sdk"
import {
  ContainerRegistrationKeys,
  Modules,
  generateEntityId,
} from "@medusajs/framework/utils"
import type { IInventoryService, IFileModuleService } from "@medusajs/framework/types"
import {
  acquireLockStep,
  releaseLockStep,
  adjustInventoryLevelsStep,
  createInventoryLevelsWorkflow,
  updateInventoryLevelsWorkflow,
  updateReservationsWorkflow,
  createReservationsWorkflow,
  createOrderFulfillmentWorkflow,
  createOrderShipmentWorkflow,
  beginReturnOrderWorkflow,
  cancelOrderFulfillmentWorkflow,
  requestItemReturnWorkflow,
  confirmReturnRequestWorkflow,
  beginReceiveReturnWorkflow,
  dismissItemReturnRequestWorkflow,
  confirmReturnReceiveWorkflow,
  beginExchangeOrderWorkflow,
  orderExchangeAddNewItemWorkflow,
  confirmExchangeRequestWorkflow,
  orderExchangeRequestItemReturnWorkflow,
} from "@medusajs/medusa/core-flows"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"
import { AFTER_SALES_MODULE } from "../modules/after-sales"
import AfterSalesModuleService from "../modules/after-sales/service"
import {
  requireDevice,
  matchesIdentity,
  assertVariantConfiguration,
  intakeDeviceConfiguration,
} from "../modules/devices/policy"
import type {
  DeviceCommandBody,
  IntakeDeviceBody,
} from "../api/admin/devices/validators"
import { authorizeDeviceCommand } from "../api/device-access"
import { assertDeviceIdentity } from "../modules/devices/identity"
import { orderDeviceCapacity } from "../lib/order-device-capacity"
import { orderItemQuantities } from "../lib/order-item-quantities"
import { deviceIdentityFilters, deviceRadioValues } from "../lib/device-identity"
import { devicePhotoUrl } from "../lib/device-photo-url"

type Input = Exclude<DeviceCommandBody, { action: "source" }> & {
  actor_id: string
}
type Table =
  | "Devices"
  | "DeviceSources"
  | "DeviceVariants"
  | "DeviceSupplies"
  | "DeviceAssignments"
  | "DeviceEvents"
type Mutation = {
  deleted?: boolean
  table: Table
  before?: Record<string, any>
  after: Record<string, any>
}
type Plan = {
  action: Input["action"]
  mutations: Mutation[]
  levels: {
    inventory_item_id: string
    location_id: string
    stocked_quantity: number
  }[]
  updates: {
    id: string
    stocked_quantity: number
    inventory_item_id: string
    location_id: string
  }[]
  adjustments: {
    inventory_item_id: string
    location_id: string
    adjustment: number
  }[]
  reservations: { id: string; location_id: string; quantity: number }[]
  newReservations: {
    inventory_item_id: string
    location_id: string
    quantity: number
    line_item_id: string
  }[]
  remainingReservations: {
    inventory_item_id: string
    location_id: string
    quantity: number
    line_item_id: string
  }[]
  fulfillment?: {
    order_id: string
    location_id: string
    items: { id: string; quantity: number }[]
    metadata: Record<string, unknown>
  }
  shipment?: {
    order_id: string
    fulfillment_id: string
    items: { id: string; quantity: number }[]
    labels: {
      tracking_number: string
      tracking_url: string
      label_url: string
    }[]
  }
  return?: { order_id: string; location_id: string; item_id: string }
  exchange?: {
    order_id: string
    variant_id: string
    original_assignment_id: string
    original_item_id: string
    location_id: string
    unit_price: number
  }
  cancel?: { order_id: string; fulfillment_id: string }
}

const prepareDevices = createStep(
  "prepare-devices",
  async (command: Input, { container }) => {
    let input = command
    await authorizeDeviceCommand(container, input.actor_id, input)
    const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
    const inventory: IInventoryService = container.resolve(Modules.INVENTORY)
    const query = container.resolve(ContainerRegistrationKeys.QUERY)
    const plan: Plan = {
      action: input.action,
      mutations: [],
      levels: [],
      updates: [],
      adjustments: [],
      reservations: [],
      newReservations: [],
      remainingReservations: [],
    }
    const mutate = (
      table: Table,
      after: Record<string, any>,
      before?: Record<string, any>,
    ) => plan.mutations.push({ table, after, before })
    const event = (
      action: string,
      data: Record<string, any>,
      device_id?: string,
      order_id?: string,
    ) =>
      mutate("DeviceEvents", {
        id: generateEntityId(undefined, "devt"),
        actor_id: input.actor_id,
        action,
        data,
        device_id: device_id || null,
        order_id: order_id || null,
      })
    const variant = async (id: string) => {
      const { data } = await query.graph({
        entity: "product_variant",
        fields: [
          "id",
          "sku",
          "manage_inventory",
          "allow_backorder",
          "inventory_items.inventory_item_id",
          "inventory_items.required_quantity",
          "options.value",
          "options.option.title",
          "product.title",
          "product.handle",
          "product.metadata",
          "product.type.value",
        ],
        filters: { id },
      })
      const v = data[0]
      const inventoryItem = v?.inventory_items?.[0]
      requireDevice(
        v &&
          v.manage_inventory &&
          !v.allow_backorder &&
          v.inventory_items?.length === 1 &&
          inventoryItem &&
          Number(inventoryItem.required_quantity) === 1,
        "SKU 必须开启库存、关闭超卖，并关联一个每台消耗 1 件的库存项",
      )
      return { ...v, inventory_item_id: inventoryItem.inventory_item_id }
    }
    const managed = async (id: string) => {
      const [v] = await service.listDeviceVariants({ variant_id: id })
      // Serial tracking can use existing quantity inventory without taking it over.
      return v || (await variant(id))
    }
    const level = async (variant_id: string, location_id: string) => {
      const v = await managed(variant_id)
      const [existing] = await inventory.listInventoryLevels({
        inventory_item_id: v.inventory_item_id,
        location_id,
      })
      if (
        !existing &&
        !plan.levels.some(
          (l) =>
            l.inventory_item_id === v.inventory_item_id &&
            l.location_id === location_id,
        )
      ) {
        plan.levels.push({
          inventory_item_id: v.inventory_item_id,
          location_id,
          stocked_quantity: 0,
        })
      }
      return { inventory_item_id: v.inventory_item_id, location_id, existing }
    }
    const adjust = async (
      variant_id: string,
      location_id: string,
      adjustment: number,
    ) => {
      const l = await level(variant_id, location_id)
      if (adjustment < 0)
        requireDevice(
          l.existing &&
            Number(l.existing.stocked_quantity) + adjustment >=
              Number(l.existing.reserved_quantity),
          "减少后的库存不足以覆盖已预留订单",
        )
      plan.adjustments.push({
        inventory_item_id: l.inventory_item_id,
        location_id,
        adjustment,
      })
    }
    const getOrder = async (id: string) => {
      const { data } = await query.graph({
        entity: "order",
        fields: [
          "id",
          "status",
          "sales_channel_id",
          "items.id",
          "items.unit_price",
          "items.variant_id",
          "items.quantity",
          "items.detail.quantity",
          "items.detail.fulfilled_quantity",
          "items.detail.return_received_quantity",
          "fulfillments.id",
          "fulfillments.location_id",
          "fulfillments.shipped_at",
          "fulfillments.delivered_at",
          "fulfillments.canceled_at",
          "fulfillments.items.line_item_id",
          "fulfillments.items.quantity",
          "shipping_methods.shipping_option_id",
        ],
        filters: { id },
      })
      requireDevice(
        data[0] && !["canceled", "draft"].includes(data[0].status),
        "订单不存在、已取消或尚未确认",
      )
      const order = data[0]
      order.items?.forEach((item) => {
        if (item?.detail) Object.assign(item.detail, orderItemQuantities(item))
      })
      return order
    }
    const intake = async (d: IntakeDeviceBody, supplier = false) => {
      for (const photo of d.photos || []) {
        const stored = await container.resolve<IFileModuleService>(Modules.FILE).retrieveFile(photo.id)
        requireDevice(devicePhotoUrl(stored.url) === photo.url && stored.url.includes("device-"), "DEVICE_PHOTO_INVALID")
      }
      const source = await service.retrieveDeviceSource(d.source_id)
      requireDevice(
        source.type === (supplier ? "supplier" : "self"),
        supplier ? "代发登记必须选择供应商来源" : "本仓入库请选择自有仓库",
      )
      await managed(d.variant_id)
      const v = await variant(d.variant_id)
      const configuration = intakeDeviceConfiguration(v, d)
      assertDeviceIdentity(d, v.product)
      assertVariantConfiguration(v, configuration)
      const radioValues = deviceRadioValues(d)
      requireDevice(
        !(await service.listDevices({ $or: deviceIdentityFilters(d) })).length &&
          !plan.mutations.some((m) => m.table === "Devices" && (
            m.after.serial_number === d.serial_number ||
            deviceRadioValues(m.after).some((value) => radioValues.includes(value))
          )),
        "序列号或 IMEI / EID 已存在，整批导入未执行",
      )
      const device = {
        ...d,
        ...configuration,
        id: generateEntityId(undefined, "dev"),
        serial_number: d.serial_number || null,
        imei: d.imei || null,
        imei2: d.imei2 || null,
        eid: d.eid || null,
        location_id: source.location_id,
        category: supplier ? "sale" : d.category || "sale",
        photos: d.photos || [],
        state: supplier ? "reserved" : (d.category || "sale") === "sale" ? "available" : "inspection",
      }
      mutate("Devices", device)
      if (!supplier && device.state === "available") await adjust(d.variant_id, source.location_id, 1)
      event(supplier ? "supplier_register" : "intake", { device }, device.id)
      return device
    }

    if (input.action === "order_assign") {
      const matches = await service.listDevices({ $or: deviceIdentityFilters(input) })
      requireDevice(matches.length <= 1, "号码匹配到多个设备，请核对序列号或 IMEI / EID")
      const existing = input.device_id
        ? await service.retrieveDevice(input.device_id)
        : matches[0]
      requireDevice(existing, "DEVICE_INTAKE_REQUIRED")
      requireDevice(matches.every((d) => d.id === existing.id), "序列号与 IMEI / EID 已属于不同设备，请核对后重新填写")
      requireDevice(existing.serial_number === input.serial_number, "序列号与已入库设备不一致")
      requireDevice(!input.imei || existing.imei === input.imei, "第一个 IMEI / EID 与已入库设备不一致")
      requireDevice(!input.imei2 || existing.imei2 === input.imei2, "第二个 IMEI / EID 与已入库设备不一致")
      requireDevice(!input.eid || existing.eid === input.eid, "EID 与已入库设备不一致")
      input = {
        action: "assign",
        device_id: existing.id,
        order_id: input.order_id,
        item_id: input.item_id,
        actor_id: input.actor_id,
      }
    }

    if (input.action === "enroll") {
      requireDevice(
        !(await service.listDeviceVariants({ variant_id: input.variant_id }))
          .length,
        "该 SKU 已启用设备管理",
      )
      const v = await variant(input.variant_id)
      const { data: sharing } = await query.graph({
        entity: "product_variant",
        fields: ["id", "inventory_items.inventory_item_id"],
      })
      requireDevice(
        !sharing.some(
          (other) =>
            other.id !== v.id &&
            other.inventory_items?.some(
              (item) => item?.inventory_item_id === v.inventory_item_id,
            ),
        ),
        "该库存项被多个 SKU 共用，不能按单台设备接管",
      )
      const levels = await inventory.listInventoryLevels({
        inventory_item_id: v.inventory_item_id,
      })
      requireDevice(
        levels.every((l) => Number(l.reserved_quantity) === 0),
        "该 SKU 有未完成订单预留，完成或取消后再接管",
      )
      plan.updates = levels.map((l) => ({
        id: l.id,
        inventory_item_id: l.inventory_item_id,
        location_id: l.location_id,
        stocked_quantity: 0,
      }))
      mutate("DeviceVariants", {
        id: generateEntityId(undefined, "dvar"),
        variant_id: v.id,
        inventory_item_id: v.inventory_item_id,
      })
      event("enroll", { variant_id: v.id, previous_levels: levels })
    } else if (input.action === "update") {
      const device = await service.retrieveDevice(input.device_id)
      const { action: _action, device_id: _deviceId, ...changes } = input
      // Pick validated editable fields explicitly: serial and audit identifiers never change.
      const next = { ...device, variant_id: changes.variant_id, source_id: changes.source_id,
        location_id: changes.location_id, imei: changes.imei, imei2: changes.imei2, eid: changes.eid,
        category: changes.category, state: changes.state, grade: changes.grade,
        battery_health: changes.battery_health, cost_cents: changes.cost_cents,
        currency_code: changes.currency_code, bin: changes.bin, notes: changes.notes, photos: changes.photos }
      const assignments = await service.listDeviceAssignments({ device_id: device.id, status: ["reserved", "packed", "shipped"] })
      const allocationChanged = (["variant_id", "source_id", "location_id", "state", "category"] as const).some((key) => next[key] !== device[key])
      requireDevice(!assignments.length || !allocationChanged, "DEVICE_EDIT_ALLOCATED")
      requireDevice(next.state === device.state || !["reserved", "packed", "shipped", "repair_ready"].includes(next.state), "DEVICE_EDIT_STATE")
      requireDevice(next.state !== "available" || next.category === "sale", "DEVICE_EDIT_CATEGORY")
      const source = await service.retrieveDeviceSource(next.source_id)
      const previousSource = await service.retrieveDeviceSource(device.source_id)
      const { data: locations } = await query.graph({ entity: "stock_location", fields: ["id"], filters: { id: next.location_id } })
      requireDevice(locations.length, "DEVICE_EDIT_LOCATION")
      const v = await variant(next.variant_id)
      assertDeviceIdentity(next, v.product)
      assertVariantConfiguration(v, next)
      const duplicates = await service.listDevices({ $or: deviceIdentityFilters(next) })
      requireDevice(!duplicates.some((other) => other.id !== device.id), "序列号或 IMEI / EID 已存在，整批导入未执行")
      for (const photo of next.photos) {
        const stored = await container.resolve<IFileModuleService>(Modules.FILE).retrieveFile(photo.id)
        requireDevice(devicePhotoUrl(stored.url) === photo.url && stored.url.includes("device-"), "DEVICE_PHOTO_INVALID")
      }
      const countedBefore = device.state === "available" && previousSource.type === "self"
      const countedAfter = next.state === "available" && source.type === "self"
      const stockChanged = device.variant_id !== next.variant_id || device.location_id !== next.location_id
      if (countedBefore && (!countedAfter || stockChanged)) await adjust(device.variant_id, device.location_id, -1)
      if (countedAfter && (!countedBefore || stockChanged)) await adjust(next.variant_id, next.location_id, 1)
      mutate("Devices", next, device)
      event("update", { before: device, after: next }, device.id)
    } else if (input.action === "activate") {
      const device = await service.retrieveDevice(input.device_id)
      requireDevice(device.category === "sale" && device.state === "inspection", "DEVICE_LOCKED")
      requireDevice(!(await service.listDeviceAssignments({ device_id: device.id, status: ["reserved", "packed", "shipped", "returned"] })).length, "DEVICE_HAS_SALES_HISTORY")
      await adjust(device.variant_id, device.location_id, 1)
      mutate("Devices", { ...device, state: "available" }, device)
      event("activate", { previous_state: device.state, category: device.category }, device.id)
    } else if (input.action === "delete" || input.action === "retire") {
      const device = await service.retrieveDevice(input.device_id)
      requireDevice(!["reserved", "packed", "shipped", "repair_ready"].includes(device.state), "DEVICE_LOCKED")
      const assignments = await service.listDeviceAssignments({ device_id: device.id, status: ["reserved", "packed", "shipped", "returned"] })
      requireDevice(!assignments.length, "DEVICE_HAS_SALES_HISTORY")
      if (device.state === "available") await adjust(device.variant_id, device.location_id, -1)
      if (input.action === "delete") plan.mutations.push({ table: "Devices", before: device, after: device, deleted: true })
      else mutate("Devices", { ...device, state: "retired" }, device)
      event(input.action, { previous_state: device.state, category: device.category }, device.id)
    } else if (input.action === "intake") {
      for (const d of input.devices) await intake(d)
    } else if (input.action === "supply") {
      const source = await service.retrieveDeviceSource(input.source_id)
      requireDevice(source.type === "supplier", "可供数量只能设置在供应商来源")
      await variant(input.variant_id)
      const [existing] = await service.listDeviceSupplies({
        source_id: source.id,
        variant_id: input.variant_id,
      })
      await adjust(
        input.variant_id,
        source.location_id,
        input.quantity - (existing?.quantity || 0),
      )
      mutate(
        "DeviceSupplies",
        {
          id: existing?.id || generateEntityId(undefined, "dsup"),
          source_id: source.id,
          variant_id: input.variant_id,
          quantity: input.quantity,
        },
        existing,
      )
      event("supply", {
        source_id: source.id,
        variant_id: input.variant_id,
        previous: existing?.quantity || 0,
        quantity: input.quantity,
      })
    } else if (
      input.action === "assign" ||
      input.action === "supplier_assign" ||
      input.action === "save_allocation"
    ) {
      requireDevice(input.action !== "supplier_assign", "DEVICE_INTAKE_REQUIRED")
      type AllocationInput = Extract<Input, { action: "assign" }> & { assignment_id?: string; fulfillment_id?: string }
      const allocations: AllocationInput[] = input.action === "save_allocation"
        ? input.allocations.map((entry) => ({ ...entry, action: "assign", order_id: input.order_id, actor_id: input.actor_id }))
        : [input]
      requireDevice(new Set(allocations.map((entry) => entry.device_id)).size === allocations.length, "设备不能重复选择")
      const changing = allocations.flatMap((entry) => entry.assignment_id ? [entry.assignment_id] : [])
      requireDevice(new Set(changing).size === changing.length, "设备不能重复选择")
      for (const allocation of allocations) {
      const order = await getOrder(allocation.order_id)
      const item = order.items?.find((i) => i?.id === allocation.item_id)
      requireDevice(item?.variant_id, "商品不属于该订单")
      let replacementRequest: string | undefined
      if (allocation.replacement_of) {
        const original = await service.retrieveDeviceAssignment(
          allocation.replacement_of,
        )
        requireDevice(
          original.order_id === order.id &&
            original.item_id === item.id &&
            original.status === "returned" &&
            original.after_sales_request_id,
          "换机必须关联本订单已退回的原机记录",
        )
        const request = await container
          .resolve<AfterSalesModuleService>(AFTER_SALES_MODULE)
          .retrieveAfterSalesRequest(original.after_sales_request_id)
        requireDevice(
          request.type === "exchange" && request.status === "processing",
          "原机没有正在处理的换机申请",
        )
        requireDevice(
          !(
            await service.listDeviceAssignments({
              replacement_of: original.id,
              status: ["reserved", "packed", "shipped"],
            })
          ).length,
          "该原机已配过替换设备",
        )
        replacementRequest = request.id
        const originalDevice = await service.retrieveDevice(original.device_id)
        requireDevice(!original.return_id, "原机已关联其他原生退货流程")
        plan.exchange = {
          order_id: order.id,
          variant_id: item.variant_id,
          original_assignment_id: original.id,
          original_item_id: item.id,
          location_id: originalDevice.location_id,
          unit_price: Number(item.unit_price),
        }
        mutate("DeviceAssignments", { ...original }, original)
      }
      const storedAssignments = await service.listDeviceAssignments({ order_id: order.id, item_id: item.id, status: ["reserved", "packed", "shipped"] })
      const staged = plan.mutations.filter((entry) => entry.table === "DeviceAssignments" && entry.after.order_id === order.id && entry.after.item_id === item.id)
      const assignments = [...storedAssignments.filter((entry) => !staged.some((change) => change.after.id === entry.id)), ...staged.map((entry) => entry.after)]
      const previous = allocation.assignment_id ? await service.retrieveDeviceAssignment(allocation.assignment_id) : undefined
      if (previous) {
        requireDevice(previous.order_id === order.id && previous.item_id === item.id && ["reserved", "packed"].includes(previous.status), "DEVICE_ALLOCATION_LOCKED")
        if (previous.fulfillment_id) {
          const fulfillment = order.fulfillments?.find((f) => f?.id === previous.fulfillment_id)
          requireDevice(fulfillment && !fulfillment.shipped_at && !fulfillment.delivered_at && !fulfillment.canceled_at, "DEVICE_ALLOCATION_LOCKED")
        }
        if (previous.device_id === allocation.device_id) continue
      }
      const capacity = orderDeviceCapacity(item, assignments as any, (order.fulfillments || []) as any)
      if (!allocation.replacement_of && !previous) requireDevice(capacity.remaining > 0, "该订单商品已配足或已履约")
      const device = await service.retrieveDevice(allocation.device_id)
      assertDeviceIdentity(device, (await variant(item.variant_id)).product)
      requireDevice(
        device.variant_id === item.variant_id,
        "设备 SKU 与订单不匹配",
      )
      requireDevice(device.state === "available" && device.category === "sale", "只有可售设备才能配货")
      requireDevice(
        !(
          await service.listDeviceAssignments({
            device_id: device.id,
            status: ["reserved", "packed", "shipped"],
          })
        ).length,
        "该设备已关联未结束的销售记录",
      )
      const { data: locationData } = await query.graph({
        entity: "stock_location",
        fields: ["sales_channels.id"],
        filters: { id: device.location_id },
      })
      requireDevice(
        locationData[0]?.sales_channels?.some(
          (c) => c?.id === order.sales_channel_id,
        ),
        "供货来源未关联订单销售渠道",
      )
      const slot = !previous && !allocation.replacement_of && (allocation.fulfillment_id || capacity.unfulfilled === 0)
        ? capacity.slots.find((f) => (!allocation.fulfillment_id || f.id === allocation.fulfillment_id) && f.location_id === device.location_id)
        : undefined
      if (!previous && !allocation.replacement_of && (allocation.fulfillment_id || capacity.unfulfilled === 0)) requireDevice(slot, "DEVICE_FULFILLMENT_LOCATION")
      if (previous) {
        const oldDevice = await service.retrieveDevice(previous.device_id)
        requireDevice(oldDevice.state === previous.status, "设备状态已改变")
        if (previous.status === "packed") requireDevice(oldDevice.location_id === device.location_id && oldDevice.source_id === device.source_id, "DEVICE_FULFILLMENT_LOCATION")
        mutate("Devices", { ...oldDevice, state: "available" }, oldDevice)
        event("replace_allocation", { assignment_id: previous.id, previous_device_id: oldDevice.id, device_id: device.id }, oldDevice.id, order.id)
      }
      const status = previous?.status || (slot ? "packed" : "reserved")
      mutate("Devices", { ...device, state: status }, device)
      const currentSource = (
        await service.listDeviceSources({ location_id: device.location_id })
      )[0]
      requireDevice(currentSource, "设备所在仓库没有对应供货来源")
      const assignment = {
        ...previous,
        id: previous?.id || generateEntityId(undefined, "dasg"),
        device_id: device.id,
        order_id: order.id,
        item_id: item.id,
        source_id: currentSource.id,
        status,
        fulfillment_id: previous?.fulfillment_id || slot?.id || null,
        replacement_of: previous?.replacement_of || allocation.replacement_of || null,
        after_sales_request_id: previous?.after_sales_request_id || replacementRequest || null,
      }
      mutate("DeviceAssignments", assignment, previous)
      event("assign", { assignment }, device.id, order.id)
      }
    } else if (input.action === "release") {
      const a = await service.retrieveDeviceAssignment(input.assignment_id)
      requireDevice(["reserved", "packed"].includes(a.status), "只有未发货的配货可以解除")
      await getOrder(a.order_id)
      const device = await service.retrieveDevice(a.device_id)
      requireDevice(device.state === a.status, "设备状态已改变")
      if (a.status === "packed") {
        requireDevice(a.fulfillment_id, "履约不存在或已发货")
        const { data: fulfillments } = await query.graph({
          entity: "fulfillment",
          fields: ["id", "shipped_at", "delivered_at", "canceled_at"],
          filters: { id: a.fulfillment_id },
        })
        const fulfillment = fulfillments[0]
        requireDevice(fulfillment && !fulfillment.shipped_at && !fulfillment.delivered_at && !fulfillment.canceled_at, "履约不存在或已发货")
        const assignments = await service.listDeviceAssignments({ fulfillment_id: a.fulfillment_id })
        requireDevice(assignments.some((entry) => entry.id === a.id) && assignments.every((entry) =>
          entry.status === "packed" && entry.order_id === a.order_id && entry.source_id === a.source_id,
        ), "只有尚未发出的履约可以取消")
        // Native fulfillment cancellation restores the whole parcel's inventory.
        // Keep its other devices allocated so only the selected device is released.
        for (const other of assignments) {
          if (other.id === a.id) continue
          const packedDevice = await service.retrieveDevice(other.device_id)
          requireDevice(packedDevice.state === "packed", "设备状态已改变")
          mutate("Devices", { ...packedDevice, state: "reserved" }, packedDevice)
          mutate("DeviceAssignments", { ...other, status: "reserved", fulfillment_id: null }, other)
          event("cancel_fulfillment", { fulfillment_id: a.fulfillment_id }, packedDevice.id, a.order_id)
        }
        plan.cancel = { order_id: a.order_id, fulfillment_id: a.fulfillment_id }
      }
      mutate("DeviceAssignments", { ...a, status: "canceled", fulfillment_id: null }, a)
      mutate("Devices", { ...device, state: "available" }, device)
      event("release", { assignment_id: a.id, fulfillment_id: a.fulfillment_id }, device.id, a.order_id)
    } else if (input.action === "fulfill") {
      requireDevice(
        new Set(input.assignment_ids).size === input.assignment_ids.length,
        "设备不能重复选择",
      )
      const assignments = await service.listDeviceAssignments({
        id: input.assignment_ids,
      })
      requireDevice(
        assignments.length === input.assignment_ids.length &&
          assignments.every((a) => a.status === "reserved"),
        "存在未配货或已履约的设备",
      )
      const first = assignments[0]
      requireDevice(
        assignments.every(
          (a) =>
            a.order_id === first.order_id && a.source_id === first.source_id,
        ),
        "一次履约必须来自同一订单和同一供货来源；混合订单请分批履约",
      )
      const order = await getOrder(first.order_id)
      const source = await service.retrieveDeviceSource(first.source_id)
      requireDevice(
        order.shipping_methods?.[0]?.shipping_option_id,
        "订单缺少配送方式，先在订单中添加配送方式",
      )
      const items: { id: string; quantity: number }[] = []
      for (const a of assignments) {
        const d = await service.retrieveDevice(a.device_id)
        assertDeviceIdentity(d, (await variant(d.variant_id)).product)
        requireDevice(
          d.state === "reserved" && d.location_id === source.location_id,
          "设备位置或状态已变化",
        )
        const item = items.find((i) => i.id === a.item_id)
        if (item) item.quantity += 1
        else items.push({ id: a.item_id, quantity: 1 })
        mutate("Devices", { ...d, state: "packed" }, d)
        mutate("DeviceAssignments", { ...a, status: "packed" }, a)
        event(
          "fulfill",
          { assignment_id: a.id, source_id: source.id },
          d.id,
          a.order_id,
        )
      }
      for (const item of items) {
        const orderItem = order.items?.find((i) => i?.id === item.id)
        requireDevice(
          orderItem?.variant_id &&
            item.quantity <=
              Number(orderItem.detail?.quantity) -
                Number(orderItem.detail?.fulfilled_quantity || 0),
          "履约数量超过待发数量",
        )
        const v = await managed(orderItem.variant_id)
        const reservations = await inventory.listReservationItems({
          line_item_id: item.id,
        })
        requireDevice(
          reservations.length <= 1 &&
            reservations.every(
              (r) => r.inventory_item_id === v.inventory_item_id,
            ),
          "订单存在多笔库存预留，请先在后台合并预留",
        )
        const [r] = reservations
        const [l] = await inventory.listInventoryLevels({
          inventory_item_id: v.inventory_item_id,
          location_id: source.location_id,
        })
        requireDevice(
          l &&
            Number(l.stocked_quantity) -
              Number(l.reserved_quantity) +
              (r?.location_id === source.location_id
                ? Number(r.quantity)
                : 0) >=
              item.quantity,
          "该来源可用库存不足以完成本次履约",
        )
        if (r) {
          requireDevice(Number(r.quantity) >= item.quantity, "订单库存预留不足")
          plan.reservations.push({
            id: r.id,
            location_id: source.location_id,
            quantity: item.quantity,
          })
          if (Number(r.quantity) > item.quantity)
            plan.remainingReservations.push({
              inventory_item_id: v.inventory_item_id,
              location_id: r.location_id,
              quantity: Number(r.quantity) - item.quantity,
              line_item_id: item.id,
            })
        } else
          plan.newReservations.push({
            inventory_item_id: v.inventory_item_id,
            location_id: source.location_id,
            quantity: item.quantity,
            line_item_id: item.id,
          })
      }
      plan.fulfillment = {
        order_id: order.id,
        location_id: source.location_id,
        items,
        metadata: {
          device_assignment_ids: assignments.map((a) => a.id),
          device_source_id: source.id,
        },
      }
    } else if (input.action === "cancel_fulfillment") {
      const assignments = await service.listDeviceAssignments({
        fulfillment_id: input.fulfillment_id,
      })
      requireDevice(
        assignments.length && assignments.every((a) => a.status === "packed"),
        "只有尚未发出的履约可以取消",
      )
      for (const a of assignments) {
        const d = await service.retrieveDevice(a.device_id)
        requireDevice(d.state === "packed", "设备状态已改变")
        mutate("Devices", { ...d, state: "reserved" }, d)
        mutate(
          "DeviceAssignments",
          { ...a, status: "reserved", fulfillment_id: null },
          a,
        )
        event(
          "cancel_fulfillment",
          { fulfillment_id: input.fulfillment_id },
          d.id,
          a.order_id,
        )
      }
      plan.cancel = {
        order_id: assignments[0].order_id,
        fulfillment_id: input.fulfillment_id,
      }
    } else if (input.action === "ship") {
      const assignments = await service.listDeviceAssignments({
        fulfillment_id: input.fulfillment_id,
      })
      requireDevice(
        assignments.length && assignments.every((a) => a.status === "packed"),
        "履约不存在或已发货",
      )
      await getOrder(assignments[0].order_id)
      const items: { id: string; quantity: number }[] = []
      for (const a of assignments) {
        const d = await service.retrieveDevice(a.device_id)
        assertDeviceIdentity(d, (await variant(d.variant_id)).product)
        requireDevice(d.state === "packed", "设备未处于待发货状态")
        const i = items.find((i) => i.id === a.item_id)
        if (i) i.quantity += 1
        else items.push({ id: a.item_id, quantity: 1 })
        mutate("Devices", { ...d, state: "shipped" }, d)
        mutate(
          "DeviceAssignments",
          { ...a, status: "shipped", tracking_number: input.tracking_number },
          a,
        )
        event(
          "ship",
          { assignment_id: a.id, tracking_number: input.tracking_number },
          d.id,
          a.order_id,
        )
      }
      plan.shipment = {
        order_id: assignments[0].order_id,
        fulfillment_id: input.fulfillment_id,
        items,
        labels: [
          {
            tracking_number: input.tracking_number,
            tracking_url: input.tracking_url || "",
            label_url: "",
          },
        ],
      }
    } else if (input.action === "receive") {
      const a = await service.retrieveDeviceAssignment(input.assignment_id)
      const d = await service.retrieveDevice(a.device_id)
      requireDevice(
        a.status === "shipped" && d.state === "shipped",
        "设备不是已发货状态，不能重复收回",
      )
      requireDevice(
        matchesIdentity(input.scan, d),
        "扫描的序列号 / IMEI / EID 与订单发出的机器不符",
      )
      const afterSales: AfterSalesModuleService =
        container.resolve(AFTER_SALES_MODULE)
      const request = await afterSales.retrieveAfterSalesRequest(
        input.request_id,
      )
      const lines = (
        request.items as {
          lines: { item_id: string; quantity: number; device_ids?: string[] }[]
        }
      ).lines
      const line = lines.find((i) => i.item_id === a.item_id)
      requireDevice(
        request.order_id === a.order_id &&
          request.status === "processing" &&
          line &&
          (!line.device_ids?.length || line.device_ids.includes(d.id)),
        "请先审核该设备对应的售后申请并标记处理中",
      )
      const received = await service.listDeviceAssignments({
        after_sales_request_id: request.id,
        item_id: a.item_id,
      })
      requireDevice(
        received.length < line.quantity,
        "已收回数量达到本次申请数量",
      )
      const source = await service.retrieveDeviceSource(input.source_id)
      mutate(
        "Devices",
        {
          ...d,
          location_id: source.location_id,
          state: request.type === "repair" ? "repair" : "inspection",
          category: "returned",
        },
        d,
      )
      mutate(
        "DeviceAssignments",
        { ...a, status: "returned", after_sales_request_id: request.id },
        a,
      )
      if (request.type === "return")
        plan.return = {
          order_id: a.order_id,
          location_id: source.location_id,
          item_id: a.item_id,
        }
      event(
        "receive",
        {
          assignment_id: a.id,
          request_id: request.id,
          type: request.type,
          destination: source.id,
        },
        d.id,
        a.order_id,
      )
    } else if (input.action === "inspect") {
      const d = await service.retrieveDevice(input.device_id)
      requireDevice(d.category === "returned" || d.category === "recycled", "DEVICE_INSPECTION_CATEGORY")
      requireDevice(
        ["inspection", "repair"].includes(d.state),
        "设备必须处于待检测或维修状态",
      )
      const variant_id = input.variant_id || d.variant_id
      await managed(variant_id)
      const source = (
        await service.listDeviceSources({ location_id: d.location_id })
      )[0]
      requireDevice(source, "设备当前位置未配置来源")
      const [a] = await service.listDeviceAssignments(
        { device_id: d.id, status: "returned" },
        { order: { created_at: "DESC" }, take: 1 },
      )
      let repair = false
      if (a?.after_sales_request_id) {
        const request = await container
          .resolve<AfterSalesModuleService>(AFTER_SALES_MODULE)
          .retrieveAfterSalesRequest(a.after_sales_request_id)
        repair = request.type === "repair" && request.status === "processing"
      }
      const state =
        input.result === "pass"
          ? repair
            ? "repair_ready"
            : "available"
          : input.result === "repair"
            ? "repair"
            : input.result === "retire"
              ? "retired"
              : "supplier_returned"
      if (input.result === "pass") {
        assertVariantConfiguration(await variant(variant_id), input)
        if (repair)
          requireDevice(
            variant_id === d.variant_id,
            "原机维修返还不能更改商品 SKU",
          )
        else await adjust(variant_id, d.location_id, 1)
      }
      if (input.result === "supplier_return")
        requireDevice(
          (await service.retrieveDeviceSource(d.source_id)).type === "supplier",
          "设备原始来源不是供应商",
        )
      mutate(
        "Devices",
        {
          ...d,
          variant_id,
          state,
          category: state === "available" ? "sale" : d.category,
          grade: input.grade,
          battery_health: input.battery_health,
          notes: input.notes,
        },
        d,
      )
      event(
        "inspect",
        {
          result: input.result,
          notes: input.notes,
          before: {
            state: d.state,
            variant_id: d.variant_id,
            grade: d.grade,
            battery_health: d.battery_health,
          },
          after: {
            state,
            variant_id,
            grade: input.grade,
            battery_health: input.battery_health,
          },
        },
        d.id,
        a?.order_id,
      )
    } else if (input.action === "repair_ship") {
      const a = await service.retrieveDeviceAssignment(input.assignment_id)
      const d = await service.retrieveDevice(a.device_id)
      requireDevice(
        a.status === "returned" &&
          d.state === "repair_ready" &&
          a.after_sales_request_id,
        "请先完成原机维修检测",
      )
      await getOrder(a.order_id)
      const request = await container
        .resolve<AfterSalesModuleService>(AFTER_SALES_MODULE)
        .retrieveAfterSalesRequest(a.after_sales_request_id)
      requireDevice(
        request.type === "repair" && request.status === "processing",
        "维修申请未处于处理中",
      )
      requireDevice(
        !(
          await service.listDeviceAssignments({
            device_id: d.id,
            status: ["reserved", "packed", "shipped"],
          })
        ).length,
        "设备已关联其他订单",
      )
      mutate("Devices", { ...d, state: "shipped" }, d)
      mutate(
        "DeviceAssignments",
        { ...a, status: "shipped", tracking_number: input.tracking_number },
        a,
      )
      event(
        "repair_ship",
        {
          assignment_id: a.id,
          request_id: request.id,
          tracking_number: input.tracking_number,
        },
        d.id,
        a.order_id,
      )
    }
    return new StepResponse(plan)
  },
)

// All module mutations have snapshots. Failed writes and later workflow failures restore them.
const writeDevices = createStep(
  "write-devices",
  async (
    input: {
      plan: Plan
      fulfillment_id?: string
      return_id?: string
      exchange_item_id?: string
      exchange_return_id?: string
    },
    { container },
  ) => {
    const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
    const done: Mutation[] = []
    try {
      for (const mutation of input.plan.mutations) {
        const after = { ...mutation.after }
        if (mutation.table === "DeviceAssignments" && input.fulfillment_id)
          after.fulfillment_id = input.fulfillment_id
        if (mutation.table === "DeviceAssignments" && input.return_id)
          after.return_id = input.return_id
        if (
          mutation.table === "DeviceAssignments" &&
          input.exchange_item_id &&
          after.replacement_of
        )
          after.item_id = input.exchange_item_id
        if (
          mutation.table === "DeviceAssignments" &&
          input.exchange_return_id &&
          after.id === input.plan.exchange?.original_assignment_id
        )
          after.return_id = input.exchange_return_id
        if (mutation.deleted) await service.softDeleteDevices(after.id)
        else await write(service, mutation.table, after, Boolean(mutation.before))
        done.push(mutation)
      }
    } catch (error) {
      await undo(service, done)
      throw error
    }
    return new StepResponse(
      {
        action: input.plan.action,
        device_ids: input.plan.mutations
          .filter((m) => m.table === "Devices")
          .map((m) => m.after.id),
        fulfillment_id: input.fulfillment_id,
        return_id: input.return_id,
      },
      done,
    )
  },
  async (mutations, { container }) => {
    if (mutations)
      await undo(
        container.resolve<DevicesModuleService>(DEVICES_MODULE),
        mutations,
      )
  },
)

function clean(record: Record<string, any>) {
  const { created_at, updated_at, deleted_at, ...data } = record
  return data
}
async function write(
  service: DevicesModuleService,
  table: Table,
  record: Record<string, any>,
  update: boolean,
) {
  const method = `${update ? "update" : "create"}${table}`
  await service[method](clean(record))
}
async function undo(service: DevicesModuleService, mutations: Mutation[]) {
  for (const m of [...mutations].reverse()) {
    if (m.deleted) { await service.restoreDevices(m.after.id); continue }
    if (m.before) await write(service, m.table, m.before, true)
    else if (m.table === "Devices") await service.softDeleteDevices(m.after.id)
    else await service[`delete${m.table}`](m.after.id)
  }
}

const resolveExchangeItem = createStep(
  "resolve-exchange-item",
  async (input: { exchange_id: string; confirmed: unknown }, { container }) => {
    const query = container.resolve(ContainerRegistrationKeys.QUERY)
    const { data } = await query.graph({
      entity: "order_exchange",
      fields: ["id", "return_id", "additional_items.item_id"],
      filters: { id: input.exchange_id },
    })
    requireDevice(data[0]?.additional_items?.length === 1, "替换商品未正确创建")
    requireDevice(data[0].return_id, "换机入库退货记录未创建")
    return new StepResponse({
      item_id: data[0].additional_items[0]!.item_id,
      return_id: data[0].return_id,
    })
  },
)

export const deviceOperationWorkflow = createWorkflow(
  "device-operation",
  (input: Input) => {
    acquireLockStep({ key: "device-operations", timeout: 30, ttl: 180 })
    const plan = prepareDevices(input)
    when(plan, (p) => p.levels.length > 0).then(() => {
      createInventoryLevelsWorkflow.runAsStep({
        input: { inventory_levels: plan.levels },
      })
    })
    when(plan, (p) => p.updates.length > 0).then(() => {
      updateInventoryLevelsWorkflow.runAsStep({
        input: { updates: plan.updates },
      })
    })
    adjustInventoryLevelsStep(plan.adjustments)
    const exchangeItem = when("device-native-exchange", plan, (p) =>
      Boolean(p.exchange),
    ).then(() => {
      const change = beginExchangeOrderWorkflow.runAsStep({
        input: transform(plan, (p) => ({
          order_id: p.exchange!.order_id,
          metadata: { replacement_of: p.exchange!.original_assignment_id },
          description: "设备售后换机（原机已收回）",
        })),
      })
      orderExchangeRequestItemReturnWorkflow.runAsStep({
        input: {
          exchange_id: change.exchange_id!,
          return_id: "",
          location_id: plan.exchange!.location_id,
          items: transform(plan, (p) => [
            { id: p.exchange!.original_item_id, quantity: 1 },
          ]),
        },
      })
      orderExchangeAddNewItemWorkflow.runAsStep({
        input: {
          exchange_id: change.exchange_id!,
          items: transform(plan, (p) => [
            {
              variant_id: p.exchange!.variant_id,
              quantity: 1,
              unit_price: p.exchange!.unit_price,
            },
          ]),
        },
      })
      const confirmed = confirmExchangeRequestWorkflow.runAsStep({
        input: { exchange_id: change.exchange_id! },
      })
      const details = resolveExchangeItem({
        exchange_id: change.exchange_id!,
        confirmed,
      })
      beginReceiveReturnWorkflow
        .runAsStep({ input: { return_id: details.return_id } })
        .config({ name: "begin-exchange-receive" })
      dismissItemReturnRequestWorkflow
        .runAsStep({
          input: {
            return_id: details.return_id,
            items: transform(plan, (p) => [
              { id: p.exchange!.original_item_id, quantity: 1 },
            ]),
          },
        })
        .config({ name: "quarantine-exchange-receive" })
      confirmReturnReceiveWorkflow
        .runAsStep({ input: { return_id: details.return_id } })
        .config({ name: "confirm-exchange-receive" })
      return details
    })
    when(plan, (p) => p.reservations.length > 0).then(() => {
      updateReservationsWorkflow.runAsStep({
        input: { updates: plan.reservations },
      })
    })
    when(plan, (p) => p.newReservations.length > 0).then(() => {
      createReservationsWorkflow.runAsStep({
        input: { reservations: plan.newReservations },
      })
    })
    const fulfillment = when(plan, (p) => Boolean(p.fulfillment)).then(() => {
      return createOrderFulfillmentWorkflow.runAsStep({
        input: transform(plan, (p) => p.fulfillment!),
      })
    })
    when(
      { plan, fulfillment },
      ({ plan }) => plan.remainingReservations.length > 0,
    ).then(() => {
      createReservationsWorkflow
        .runAsStep({ input: { reservations: plan.remainingReservations } })
        .config({ name: "restore-remaining-reservations" })
    })
    when(plan, (p) => Boolean(p.shipment)).then(() => {
      createOrderShipmentWorkflow.runAsStep({
        input: transform(plan, (p) => p.shipment!),
      })
    })
    const received = when("device-native-return", plan, (p) =>
      Boolean(p.return),
    ).then(() => {
      const change = beginReturnOrderWorkflow.runAsStep({
        input: transform(plan, (p) => ({
          order_id: p.return!.order_id,
          location_id: p.return!.location_id,
        })),
      })
      const items = transform(plan, (p) => [
        { id: p.return!.item_id, quantity: 1 },
      ])
      requestItemReturnWorkflow.runAsStep({
        input: { return_id: change.return_id!, items },
      })
      confirmReturnRequestWorkflow.runAsStep({
        input: { return_id: change.return_id! },
      })
      beginReceiveReturnWorkflow.runAsStep({
        input: { return_id: change.return_id! },
      })
      // Quarantine: receive as non-restocking. A passing inspection adds saleable stock once.
      dismissItemReturnRequestWorkflow.runAsStep({
        input: { return_id: change.return_id!, items },
      })
      confirmReturnReceiveWorkflow.runAsStep({
        input: { return_id: change.return_id! },
      })
      return change
    })
    const writeInput = transform(
      { plan, fulfillment, received, exchangeItem },
      (data) => ({
        plan: data.plan,
        fulfillment_id: data.fulfillment?.id,
        return_id: data.received?.return_id || undefined,
        exchange_item_id: data.exchangeItem?.item_id,
        exchange_return_id: data.exchangeItem?.return_id,
      }),
    )
    const result = writeDevices(writeInput)
    when({ plan, result }, ({ plan }) => Boolean(plan.cancel)).then(() => {
      cancelOrderFulfillmentWorkflow.runAsStep({
        input: transform(plan, (p) => p.cancel!),
      })
    })
    releaseLockStep({ key: "device-operations" })
    return new WorkflowResponse(result)
  },
)
