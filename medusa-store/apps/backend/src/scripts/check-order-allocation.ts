import assert from "node:assert/strict"
import type {
  ExecArgs,
  IInventoryService,
  IOrderModuleService,
  IFulfillmentModuleService,
  IUserModuleService,
} from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import {
  createProductsWorkflow,
  updateProductsWorkflow,
  deleteProductsWorkflow,
  createStockLocationsWorkflow,
  deleteStockLocationsWorkflow,
  linkSalesChannelsToStockLocationWorkflow,
} from "@medusajs/medusa/core-flows"
import { deviceOperationWorkflow } from "../workflows/device-operations"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"
import {
  DeviceCommand,
  type DeviceCommandBody,
} from "../api/admin/devices/validators"
import { orderItemQuantities } from "../lib/order-item-quantities"
import { requiresImei } from "../modules/devices/identity"
import { GET as getDevices } from "../api/admin/devices/route"
import type {
  AuthenticatedMedusaRequest,
  MedusaResponse,
} from "@medusajs/framework/http"

// Isolated fixtures verify real inventory and native fulfillment without touching store orders.
export default async function checkOrderAllocation({ container }: ExecArgs) {
  assert.deepEqual(
    orderItemQuantities({
      detail: {
        raw_quantity: { value: "3" },
        raw_fulfilled_quantity: { value: "1" },
      },
    }),
    { quantity: 3, fulfilled_quantity: 1 },
  )
  assert.deepEqual(
    orderItemQuantities({ quantity: 4, detail: { fulfilled_quantity: "2" } }),
    { quantity: 4, fulfilled_quantity: 2 },
  )
  assert.deepEqual(
    orderItemQuantities({
      detail: { quantity: "invalid", fulfilled_quantity: undefined },
    }),
    { quantity: 0, fulfilled_quantity: 0 },
  )
  assert.equal(requiresImei({ title: "Apple iPhone 15" }), true)
  assert.equal(requiresImei({ title: "Tablet" }), false)
  assert.equal(
    requiresImei({
      title: "iPhone accessory",
      metadata: { requires_imei: false },
    }),
    false,
  )
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const devices: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const inventory: IInventoryService = container.resolve(Modules.INVENTORY)
  const orders: IOrderModuleService = container.resolve(Modules.ORDER)
  const fulfillment: IFulfillmentModuleService = container.resolve(
    Modules.FULFILLMENT,
  )
  const users: IUserModuleService = container.resolve(Modules.USER)
  const [user] = await users.listUsers({})
  assert(user)
  const { data: stores } = await query.graph({
    entity: "store",
    fields: ["default_sales_channel_id"],
  })
  const channelId = stores[0].default_sales_channel_id!
  const { data: options } = await query.graph({
    entity: "shipping_option",
    fields: ["id", "shipping_profile_id", "provider_id"],
  })
  const shipping = options.find((s) => s.provider_id === "manual_manual")!
  assert(shipping)
  const stamp = String(Date.now())
  let productId = "",
    variantId = "",
    locationId = "",
    inventoryId = ""
  const orderIds: string[] = []
  const run = (body: DeviceCommandBody) => {
    const parsed = DeviceCommand.parse(body)
    assert(parsed.action !== "source")
    return deviceOperationWorkflow(container).run({
      input: { ...parsed, actor_id: user.id },
    })
  }
  const stock = async () =>
    Number(
      (
        await inventory.listInventoryLevels({
          inventory_item_id: inventoryId,
          location_id: locationId,
        })
      )[0]?.stocked_quantity,
    )
  try {
    const { result: products } = await createProductsWorkflow(container).run({
      input: {
        products: [
          {
            title: `Allocation check ${stamp}`,
            handle: `allocation-check-${stamp}`,
            status: "published",
            shipping_profile_id: shipping.shipping_profile_id,
            sales_channels: [{ id: channelId }],
            options: [{ title: "Model", values: ["Test"] }],
            variants: [
              {
                title: "Test",
                sku: `ALLOC-${stamp}`,
                options: { Model: "Test" },
                manage_inventory: true,
                allow_backorder: false,
                prices: [{ currency_code: "eur", amount: 100 }],
              },
            ],
          },
        ],
      },
    })
    productId = products[0].id
    variantId = products[0].variants![0].id
    const { data: variants } = await query.graph({
      entity: "product_variant",
      fields: ["inventory_items.inventory_item_id"],
      filters: { id: variantId },
    })
    inventoryId = variants[0].inventory_items![0]!.inventory_item_id!
    const { result: locations } = await createStockLocationsWorkflow(
      container,
    ).run({ input: { locations: [{ name: `Allocation fixture ${stamp}` }] } })
    locationId = locations[0].id
    await linkSalesChannelsToStockLocationWorkflow(container).run({
      input: { id: locationId, add: [channelId] },
    })
    // Fixture location uses the same manual provider as a normal shipping warehouse.
    await container.resolve(ContainerRegistrationKeys.LINK).create({
      [Modules.STOCK_LOCATION]: { stock_location_id: locationId },
      [Modules.FULFILLMENT]: { fulfillment_provider_id: "manual_manual" },
    })
    await inventory.createInventoryLevels({
      inventory_item_id: inventoryId,
      location_id: locationId,
      stocked_quantity: 3,
    })
    const makeOrder = async (quantity: number) => {
      const order = await orders.createOrders({
        currency_code: "eur",
        email: `allocation-${stamp}@example.invalid`,
        sales_channel_id: channelId,
        shipping_address: {
          first_name: "Allocation",
          last_name: "Check",
          address_1: "Test 1",
          city: "Berlin",
          country_code: "de",
          postal_code: "10115",
        },
        items: [
          {
            title: "Allocation fixture",
            variant_id: variantId,
            quantity,
            unit_price: 100,
            requires_shipping: true,
          },
        ],
        shipping_methods: [
          {
            name: "Fixture shipping",
            shipping_option_id: shipping.id,
            amount: 0,
          },
        ],
      })
      orderIds.push(order.id)
      return { orderId: order.id, itemId: order.items![0].id }
    }
    const first = await makeOrder(2)
    // Preserve an existing native reservation: allocation must neither reset nor add stock.
    await inventory.createReservationItems({
      inventory_item_id: inventoryId,
      location_id: locationId,
      quantity: 2,
      line_item_id: first.itemId,
    })
    const allocate = (serial_number: string, order = first, imei?: string, imei2?: string) =>
      run({
        action: "order_assign",
        serial_number,
        ...(imei ? { imei } : {}),
        ...(imei2 ? { imei2 } : {}),
        order_id: order.orderId,
        item_id: order.itemId,
      })
    const serial = `A${stamp}`
    const imei = `86${stamp}`
    await allocate(serial)
    assert.equal(await stock(), 3)
    assert.equal(
      (await devices.listDeviceVariants({ variant_id: variantId })).length,
      0,
      "No SKU takeover",
    )
    const [device] = await devices.listDevices({ serial_number: serial })
    assert(device)
    assert.equal(device.grade, null)
    assert.equal(device.battery_health, null)
    assert.equal(
      (await inventory.listReservationItems({ line_item_id: first.itemId }))[0]
        .quantity,
      2,
    )
    await assert.rejects(allocate(serial), (error) =>
      /可售|销售记录/.test(JSON.stringify(error)),
    )
    await updateProductsWorkflow(container).run({
      input: {
        products: [
          { id: productId, title: `iPhone allocation check ${stamp}` },
        ],
      },
    })
    let listing: any
    await getDevices(
      {
        scope: container,
        auth_context: { actor_id: user.id },
        query: { order_id: first.orderId },
      } as unknown as AuthenticatedMedusaRequest,
      {
        json: (data: unknown) => {
          listing = JSON.parse(JSON.stringify(data))
        },
      } as MedusaResponse,
    )
    assert.equal(listing.order.items[0].quantity, 2)
    assert.equal(listing.order.items[0].detail.quantity, 2)
    assert.equal(listing.order.items[0].requires_imei, true)
    await assert.rejects(allocate(`M${stamp}`), (error) =>
      /iPhone/.test(JSON.stringify(error)),
    )
    const [firstAssignment] = await devices.listDeviceAssignments({
      device_id: device.id,
      status: "reserved",
    })
    await run({ action: "release", assignment_id: firstAssignment.id })
    await assert.rejects(
      run({
        action: "assign",
        device_id: device.id,
        order_id: first.orderId,
        item_id: first.itemId,
      }),
      (error) => /iPhone/.test(JSON.stringify(error)),
    )
    const eid = `8904903200000000000${stamp}`
    assert.equal(eid.length, 32)
    await allocate(serial, first, `87${stamp}`, eid)
    assert.equal((await devices.retrieveDevice(device.id)).imei, `87${stamp}`)
    assert.equal((await devices.retrieveDevice(device.id)).imei2, eid)
    for (const q of [serial, `87${stamp}`, eid]) {
      let found: any
      await getDevices({ scope: container, auth_context: { actor_id: user.id }, query: { q } } as unknown as AuthenticatedMedusaRequest,
        { json: (data: unknown) => { found = data } } as MedusaResponse)
      assert(found.devices.some((entry: any) => entry.id === device.id), "All three identity fields locate the same device")
    }
    await assert.rejects(allocate(`CROSS${stamp}`, first, undefined, `87${stamp}`),
      (error) => /不一致|不同设备|多个设备/.test(JSON.stringify(error)))
    await assert.rejects(allocate(`CROSS${stamp}`, first, eid),
      (error) => /不一致|不同设备|多个设备/.test(JSON.stringify(error)))

    await allocate(`D${stamp}`, first, imei)
    assert.equal((await devices.listDevices({ imei })).length, 1)
    await assert.rejects(allocate(`X${stamp}`, first, imei), (error) =>
      /不一致|不同设备|多个设备/.test(JSON.stringify(error)),
    )
    await assert.rejects(allocate(serial, first, imei), (error) =>
      /不一致|不同设备|多个设备/.test(JSON.stringify(error)),
    )
    const second = await makeOrder(1)
    await assert.rejects(allocate(serial, second), (error) =>
      /可售|销售记录/.test(JSON.stringify(error)),
    )
    const assignments = await devices.listDeviceAssignments({
      order_id: first.orderId,
      status: "reserved",
    })
    assert.equal(assignments.length, 2)
    await run({ action: "release", assignment_id: assignments[0].id })
    await run({
      action: "assign",
      device_id: assignments[0].device_id,
      order_id: first.orderId,
      item_id: first.itemId,
    })
    assert.equal(
      await stock(),
      3,
      "Choosing a device already in stock does not add inventory",
    )
    const current = await devices.listDeviceAssignments({
      order_id: first.orderId,
      status: "reserved",
    })
    const { result: packed } = await run({
      action: "fulfill",
      assignment_ids: current.map((a) => a.id),
    })
    assert.equal(
      await stock(),
      1,
      "Native fulfillment deducts the selected devices exactly once",
    )
    await run({
      action: "cancel_fulfillment",
      fulfillment_id: packed.fulfillment_id!,
    })
    assert.equal(await stock(), 3)
    const { result: repacked } = await run({
      action: "fulfill",
      assignment_ids: current.map((a) => a.id),
    })
    await run({
      action: "ship",
      fulfillment_id: repacked.fulfillment_id!,
      tracking_number: "ALLOC-CHECK",
    })
    assert.equal(await stock(), 1)
    await allocate(`B${stamp}`, second, `88${stamp}`)
    await assert.rejects(
      allocate(`C${stamp}`, await makeOrder(1), `89${stamp}`),
      (error) => /未登记库存/.test(JSON.stringify(error)),
    )
    assert.equal(await stock(), 1)
    assert.equal(
      (await devices.listDeviceVariants({ variant_id: variantId })).length,
      0,
    )
    const intakeEid = `8904903300000000000${stamp}`
    const intakeImei = `90${stamp}`
    const intakeDevice = { serial_number: `INTAKE${stamp}`, imei: intakeEid, imei2: intakeImei,
      source_id: device.source_id, variant_id: variantId, cost_cents: 0, currency_code: "eur", bin: "", notes: "" }
    await run({ action: "intake", devices: [intakeDevice] })
    const [storedIntake] = await devices.listDevices({ serial_number: intakeDevice.serial_number })
    assert.equal(storedIntake.imei, intakeEid)
    assert.equal(storedIntake.imei2, intakeImei)
    assert.equal(storedIntake.state, "inspection")
    assert.equal(await stock(), 1, "Intake in inspection does not increase saleable stock")
    const beforeDuplicate = (await devices.listAndCountDevices({ variant_id: variantId }))[1]
    await assert.rejects(run({ action: "intake", devices: [
      { ...intakeDevice, serial_number: `CROSS-INTAKE${stamp}`, imei: intakeImei, imei2: undefined },
    ] }), (error) => /已存在/.test(JSON.stringify(error)))
    const batchImei = `91${stamp}`
    await assert.rejects(run({ action: "intake", devices: [
      { ...intakeDevice, serial_number: `BATCH-A${stamp}`, imei: batchImei, imei2: undefined },
      { ...intakeDevice, serial_number: `BATCH-B${stamp}`, imei: undefined, imei2: batchImei },
    ] }), (error) => /已存在/.test(JSON.stringify(error)))
    assert.equal((await devices.listAndCountDevices({ variant_id: variantId }))[1], beforeDuplicate,
      "Cross-slot duplicates reject the whole batch without partial records")
    console.log(
      "PASS: serial and both IMEI/EID fields, three-field API search, cross-slot duplicate rejection, atomic batch validation, identity completion, intake, allocation, stock preservation, fulfillment and shipping.",
    )
  } finally {
    const records = variantId
      ? await devices.listDevices({ variant_id: variantId })
      : []
    if (orderIds.length) {
      const events = await devices.listDeviceEvents(
        { order_id: orderIds },
        { take: 1000 },
      )
      if (events.length)
        await devices.deleteDeviceEvents(events.map((e) => e.id))
      const assignments = await devices.listDeviceAssignments(
        { order_id: orderIds },
        { take: 1000 },
      )
      if (assignments.length)
        await devices.deleteDeviceAssignments(assignments.map((a) => a.id))
    }
    if (records.length) {
      const identityEvents = await devices.listDeviceEvents({ device_id: records.map((d) => d.id) }, { take: 1000 })
      if (identityEvents.length) await devices.deleteDeviceEvents(identityEvents.map((entry) => entry.id))
      await devices.deleteDevices(records.map((d) => d.id))
    }
    for (const orderId of orderIds) {
      const { data } = await query.graph({
        entity: "order",
        fields: ["items.id", "fulfillments.id"],
        filters: { id: orderId },
      })
      const reservations = await inventory.listReservationItems({
        line_item_id: data[0]?.items?.map((i) => i!.id) || [],
      })
      if (reservations.length)
        await inventory.deleteReservationItems(reservations.map((r) => r.id))
      for (const f of data[0]?.fulfillments || []) {
        await fulfillment.updateFulfillment(f!.id, { shipped_at: null })
        await fulfillment.cancelFulfillment(f!.id)
        await fulfillment.deleteFulfillment(f!.id)
      }
      await orders.deleteOrders(orderId)
    }
    if (productId)
      await deleteProductsWorkflow(container).run({
        input: { ids: [productId] },
      })
    if (locationId) {
      const sources = await devices.listDeviceSources({
        location_id: locationId,
      })
      if (sources.length)
        await devices.deleteDeviceSources(sources.map((s) => s.id))
      await deleteStockLocationsWorkflow(container).run({
        input: { ids: [locationId] },
      })
    }
  }
}
