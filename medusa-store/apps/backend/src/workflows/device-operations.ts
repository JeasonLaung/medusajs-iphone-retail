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
import type { IInventoryService } from "@medusajs/framework/types"
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
} from "../modules/devices/policy"
import type {
  DeviceCommandBody,
  IntakeDeviceBody,
} from "../api/admin/devices/validators"
import { authorizeDeviceCommand } from "../api/device-access"

type Input = Exclude<DeviceCommandBody, { action: "source" }> & {
  actor_id: string
}
type Table =
  | "Devices"
  | "DeviceVariants"
  | "DeviceSupplies"
  | "DeviceAssignments"
  | "DeviceEvents"
type Mutation = {
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
  async (input: Input, { container }) => {
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
      requireDevice(v, "请先在库存接管中启用该 SKU 的设备管理")
      return v
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
          "items.detail.quantity",
          "items.detail.fulfilled_quantity",
          "items.detail.return_received_quantity",
          "shipping_methods.shipping_option_id",
        ],
        filters: { id },
      })
      requireDevice(
        data[0] && !["canceled", "draft"].includes(data[0].status),
        "订单不存在、已取消或尚未确认",
      )
      return data[0]
    }
    const intake = async (d: IntakeDeviceBody, supplier = false) => {
      const source = await service.retrieveDeviceSource(d.source_id)
      requireDevice(
        source.type === (supplier ? "supplier" : "self"),
        supplier ? "代发登记必须选择供应商来源" : "本仓入库请选择自有仓库",
      )
      await managed(d.variant_id)
      assertVariantConfiguration(await variant(d.variant_id), d)
      for (const identity of ["serial_number", "imei"] as const) {
        if (!d[identity]) continue
        requireDevice(
          !(await service.listDevices({ [identity]: d[identity] })).length &&
            !plan.mutations.some(
              (m) => m.table === "Devices" && m.after[identity] === d[identity],
            ),
          "序列号或 IMEI 已存在，整批导入未执行",
        )
      }
      const device = {
        ...d,
        id: generateEntityId(undefined, "dev"),
        serial_number: d.serial_number || null,
        imei: d.imei || null,
        location_id: source.location_id,
        state: supplier ? "reserved" : "inspection",
      }
      mutate("Devices", device)
      event(supplier ? "supplier_register" : "intake", { device }, device.id)
      return device
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
      input.action === "supplier_assign"
    ) {
      const order = await getOrder(input.order_id)
      const item = order.items?.find((i) => i?.id === input.item_id)
      requireDevice(item?.variant_id, "商品不属于该订单")
      let replacementRequest: string | undefined
      if (input.replacement_of) {
        const original = await service.retrieveDeviceAssignment(
          input.replacement_of,
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
      const assignments = await service.listDeviceAssignments({
        order_id: order.id,
        item_id: item.id,
        status: ["reserved", "packed", "shipped"],
      })
      const awaiting =
        Number(item.detail?.quantity) -
        Number(item.detail?.fulfilled_quantity || 0)
      if (!input.replacement_of)
        requireDevice(
          assignments.filter((a) => a.status === "reserved").length < awaiting,
          "该订单商品已配足或已履约",
        )
      const device =
        input.action === "supplier_assign"
          ? await intake(input.device, true)
          : await service.retrieveDevice(input.device_id)
      requireDevice(
        device.variant_id === item.variant_id,
        "设备 SKU 与订单不匹配",
      )
      if (input.action === "assign")
        requireDevice(device.state === "available", "只有可售设备才能配货")
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
      if (input.action === "supplier_assign") {
        const [supply] = await service.listDeviceSupplies({
          source_id: device.source_id,
          variant_id: device.variant_id,
        })
        requireDevice(
          supply && supply.quantity > 0,
          "供应商可供数量不足，请先更新可供库存",
        )
        mutate(
          "DeviceSupplies",
          { ...supply, quantity: supply.quantity - 1 },
          supply,
        )
      } else mutate("Devices", { ...device, state: "reserved" }, device)
      const [currentSource] = await service.listDeviceSources({
        location_id: device.location_id,
      })
      requireDevice(currentSource, "设备所在仓库没有对应供货来源")
      const assignment = {
        id: generateEntityId(undefined, "dasg"),
        device_id: device.id,
        order_id: order.id,
        item_id: item.id,
        source_id: currentSource.id,
        status: "reserved",
        replacement_of: input.replacement_of || null,
        after_sales_request_id: replacementRequest || null,
      }
      mutate("DeviceAssignments", assignment)
      event("assign", { assignment }, device.id, order.id)
    } else if (input.action === "release") {
      const a = await service.retrieveDeviceAssignment(input.assignment_id)
      requireDevice(a.status === "reserved", "只有尚未创建履约的配货可以解除")
      const device = await service.retrieveDevice(a.device_id)
      mutate("DeviceAssignments", { ...a, status: "canceled" }, a)
      mutate("Devices", { ...device, state: "available" }, device)
      event("release", { assignment_id: a.id }, device.id, a.order_id)
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
        "扫描的序列号 / IMEI 与订单发出的机器不符",
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
        await write(service, mutation.table, after, Boolean(mutation.before))
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
    if (m.before) await write(service, m.table, m.before, true)
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
