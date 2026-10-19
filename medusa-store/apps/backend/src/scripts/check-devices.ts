import assert from "node:assert/strict"
import type {
  ConfigModule,
  ExecArgs,
  IOrderModuleService,
  ICustomerModuleService,
  IUserModuleService,
  IApiKeyModuleService,
  IInventoryService,
  IFulfillmentModuleService,
} from "@medusajs/framework/types"
import {
  ContainerRegistrationKeys,
  Modules,
  generateJwtToken,
} from "@medusajs/framework/utils"
import {
  createProductsWorkflow,
  deleteProductsWorkflow,
  deleteStockLocationsWorkflow,
  createOrUpdateOrderPaymentCollectionWorkflow,
  markPaymentCollectionAsPaid,
} from "@medusajs/medusa/core-flows"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"
import { AFTER_SALES_MODULE } from "../modules/after-sales"
import AfterSalesModuleService from "../modules/after-sales/service"

export default async function checkDevices({ container }: ExecArgs) {
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const afterSales: AfterSalesModuleService =
    container.resolve(AFTER_SALES_MODULE)
  const orders: IOrderModuleService = container.resolve(Modules.ORDER)
  const customers: ICustomerModuleService = container.resolve(Modules.CUSTOMER)
  const inventory: IInventoryService = container.resolve(Modules.INVENTORY)
  const fulfillment: IFulfillmentModuleService = container.resolve(
    Modules.FULFILLMENT,
  )
  const users: IUserModuleService = container.resolve(Modules.USER)
  const keys: IApiKeyModuleService = container.resolve(Modules.API_KEY)
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const config: ConfigModule = container.resolve(
    ContainerRegistrationKeys.CONFIG_MODULE,
  )
  const [user] = await users.listUsers({})
  const [key] = await keys.listApiKeys({ type: "publishable" })
  assert(user && key)
  const token = (actor_id: string, actor_type = "user") =>
    generateJwtToken(
      {
        actor_id,
        actor_type,
        auth_identity_id: "devices_check",
        app_metadata: {
          [actor_type === "user" ? "user_id" : "customer_id"]: actor_id,
        },
      },
      { secret: config.projectConfig.http.jwtSecret, expiresIn: "30m" },
    )
  const admin = token(user.id)
  const stamp = String(Date.now())
  const customer = await customers.createCustomers({
    email: `device-${stamp}@example.invalid`,
  })
  const auth = token(customer.id, "customer")
  const sourceIds: string[] = [],
    orderIds: string[] = [],
    productIds: string[] = [],
    locationIds: string[] = []
  let variant_id = ""
  let supplierUserId = ""
  let supplierAuthId = ""
  let supplierToken = ""
  const call = async (
    path: string,
    body?: unknown,
    authorization: string | null = admin,
  ) => {
    const response = await fetch(`http://127.0.0.1:9001${path}`, {
      method: body ? "POST" : "GET",
      headers: {
        "Content-Type": "application/json",
        "x-publishable-api-key": key.token,
        ...(authorization ? { authorization: `Bearer ${authorization}` } : {}),
      },
      ...(body ? { body: JSON.stringify(body) } : {}),
    })
    const data = await response.json()
    return { status: response.status, data }
  }
  const command = async (body: Record<string, unknown>, status = 200) => {
    const response = await call("/admin/devices", body)
    assert.equal(response.status, status, JSON.stringify(response.data))
    return response.data
  }
  const stock = async (location_id: string) => {
    const [managed] = await service.listDeviceVariants({ variant_id })
    const [level] = await inventory.listInventoryLevels({
      inventory_item_id: managed.inventory_item_id,
      location_id,
    })
    return Number(level?.stocked_quantity || 0)
  }
  for (let attempt = 0; attempt < 30; attempt++) {
    try {
      if ((await fetch("http://127.0.0.1:9001/health")).ok) break
    } catch {
      /* dev watcher may still be restarting */
    }
    await new Promise((resolve) => setTimeout(resolve, 1000))
  }
  try {
    assert.equal((await call("/admin/devices", undefined, null)).status, 401)
    assert.equal((await call("/admin/devices", undefined, auth)).status, 401)
    const { data: stores } = await query.graph({
      entity: "store",
      fields: ["default_sales_channel_id"],
    })
    const channel_id = stores[0].default_sales_channel_id!
    const { data: shipping } = await query.graph({
      entity: "shipping_option",
      fields: ["id", "shipping_profile_id", "provider_id"],
    })
    const shippingOption = shipping.find(
      (s) => s.provider_id === "manual_manual",
    )!
    assert(shippingOption, "A manual shipping option is required")
    const { result: products } = await createProductsWorkflow(container).run({
      input: {
        products: [
          {
            title: `Device check ${stamp}`,
            handle: `device-check-${stamp}`,
            status: "published",
            shipping_profile_id: shippingOption.shipping_profile_id,
            sales_channels: [{ id: channel_id }],
            options: [
              { title: "Condition", values: ["A"] },
              { title: "Battery", values: ["85%"] },
            ],
            variants: [
              {
                title: "A / 85%",
                sku: `DEV-CHECK-${stamp}`,
                options: { Condition: "A", Battery: "85%" },
                manage_inventory: true,
                allow_backorder: false,
                prices: [{ currency_code: "eur", amount: 100 }],
              },
            ],
          },
        ],
      },
    })
    productIds.push(products[0].id)
    variant_id = products[0].variants![0].id
    const self = (
      await command(
        { action: "source", name: `Check warehouse ${stamp}`, type: "self" },
        201,
      )
    ).source
    sourceIds.push(self.id)
    locationIds.push(self.location_id)
    const supplier = (
      await command(
        { action: "source", name: `Check supplier ${stamp}`, type: "supplier" },
        201,
      )
    ).source
    sourceIds.push(supplier.id)
    locationIds.push(supplier.location_id)
    const supplierEmail = `supplier-check-${stamp}@example.invalid`
    const password = `Supplier-check-${stamp}-${Math.random()}`
    const createdAccount = await call("/admin/devices/supplier-accounts", {
      email: supplierEmail,
      password,
      source_id: supplier.id,
      order_ids: [],
    })
    assert.equal(
      createdAccount.status,
      201,
      JSON.stringify(createdAccount.data),
    )
    const [supplierUser] = await users.listUsers({ email: supplierEmail })
    supplierUserId = supplierUser.id
    const [identity] = await container
      .resolve(Modules.AUTH)
      .listAuthIdentities({ app_metadata: { user_id: supplierUserId } })
    supplierAuthId = identity.id
    const login = await call(
      "/auth/user/emailpass",
      { email: supplierUser.email, password },
      null,
    )
    assert.equal(login.status, 200, JSON.stringify(login.data))
    supplierToken = login.data.token
    const supplierCall = (path: string, body?: unknown) =>
      call(path, body, supplierToken)
    await command({ action: "enroll", variant_id, acknowledged: true })
    const base = {
      variant_id,
      source_id: self.id,
      grade: "A",
      battery_health: 90,
      cost_cents: 5000,
    }
    const serial = `D${stamp}A`
    const serial2 = `D${stamp}B`
    const serial3 = `D${stamp}C`
    await command({
      action: "intake",
      devices: [
        { ...base, serial_number: serial },
        { ...base, serial_number: serial2 },
        { ...base, serial_number: serial3 },
      ],
    })
    await command(
      { action: "intake", devices: [{ ...base, serial_number: serial }] },
      400,
    )
    await command(
      {
        action: "intake",
        devices: [
          { ...base, serial_number: `D${stamp}Z` },
          { ...base, serial_number: serial },
        ],
      },
      400,
    )
    assert.equal(
      (await service.listDevices({ serial_number: `D${stamp}Z` })).length,
      0,
      "Invalid batch is atomic",
    )
    assert.equal(
      await stock(self.location_id),
      0,
      "Unchecked intake not saleable",
    )
    const devices = await service.listDevices(
      { variant_id },
      { order: { serial_number: "ASC" } },
    )
    const [a, b, c] = devices
    for (const d of devices)
      await command({
        action: "inspect",
        device_id: d.id,
        result: "pass",
        grade: "A",
        battery_health: 90,
        notes: "Intake QC passed",
      })
    assert.equal(await stock(self.location_id), 3)
    await command(
      {
        action: "inspect",
        device_id: a.id,
        result: "pass",
        grade: "A",
        battery_health: 90,
        notes: "Duplicate check",
      },
      400,
    )
    await command({
      action: "supply",
      source_id: supplier.id,
      variant_id,
      quantity: 3,
    })
    await command({
      action: "supply",
      source_id: supplier.id,
      variant_id,
      quantity: 3,
    })
    assert.equal(
      await stock(supplier.location_id),
      3,
      "Supply update idempotent",
    )
    const makeOrder = async (quantity: number) => {
      const order = await orders.createOrders({
        customer_id: customer.id,
        email: customer.email!,
        currency_code: "eur",
        sales_channel_id: channel_id,
        shipping_address: {
          first_name: "Device",
          last_name: "Check",
          address_1: "Test street 1",
          city: "Copenhagen",
          country_code: "dk",
          postal_code: "1000",
        },
        items: [
          {
            title: "Device check",
            variant_id,
            quantity,
            unit_price: 100,
            requires_shipping: true,
          },
        ],
        shipping_methods: [
          {
            name: "Check shipping",
            shipping_option_id: shippingOption.id,
            amount: 0,
          },
        ],
      })
      orderIds.push(order.id)
      return { order, item_id: order.items![0].id }
    }
    const mixed = await makeOrder(2)
    const accountBody = {
      email: supplierUser.email,
      source_id: supplier.id,
      order_ids: [mixed.order.id],
    }
    assert.equal(
      (await call("/admin/devices/supplier-accounts", accountBody)).status,
      200,
    )
    const supplierListing = await supplierCall("/admin/devices")
    assert.equal(
      supplierListing.status,
      200,
      JSON.stringify(supplierListing.data),
    )
    assert.equal(supplierListing.data.access.role, "supplier")
    assert.deepEqual(
      supplierListing.data.sources.map((s: any) => s.id),
      [supplier.id],
    )
    assert(
      supplierListing.data.devices.every(
        (d: any) => d.source_id === supplier.id,
      ),
    )
    assert(
      supplierListing.data.supplies.every(
        (s: any) => s.source_id === supplier.id,
      ),
    )
    for (const variant of supplierListing.data.variants)
      for (const item of variant.inventory_items || [])
        assert(
          (item.inventory?.location_levels || []).every(
            (level: any) => level.location_id === supplier.location_id,
          ),
        )
    for (const path of [
      "/admin/orders",
      "/admin/users",
      "/admin/products",
      "/admin/api-keys",
      "/admin/devices/supplier-accounts",
      `/admin/devices?device_id=${a.id}`,
    ])
      assert.equal((await supplierCall(path)).status, 403, path)
    assert.equal(
      (
        await supplierCall(`/admin/users/${supplierUser.id}`, {
          metadata: { device_role: "admin" },
        })
      ).status,
      403,
    )
    assert.equal(
      (
        await supplierCall("/admin/devices/refunds", {
          request_id: "fake",
          payment_id: "fake",
          amount: 1,
          confirmed: true,
        })
      ).status,
      403,
    )
    assert.equal(
      (
        await supplierCall("/admin/devices", {
          action: "source",
          name: "denied",
          type: "supplier",
        })
      ).status,
      403,
    )
    assert.equal(
      (
        await supplierCall("/admin/devices", {
          action: "enroll",
          variant_id,
          acknowledged: true,
        })
      ).status,
      403,
    )
    assert.equal(
      (
        await supplierCall("/admin/devices", {
          action: "supply",
          source_id: self.id,
          variant_id,
          quantity: 3,
        })
      ).status,
      403,
    )
    assert.equal(
      (
        await supplierCall("/admin/devices", {
          action: "supply",
          source_id: supplier.id,
          variant_id,
          quantity: 3,
        })
      ).status,
      200,
    )
    const { result: paycol } =
      await createOrUpdateOrderPaymentCollectionWorkflow(container).run({
        input: { order_id: mixed.order.id },
      })
    assert(paycol)
    const { result: payment } = await markPaymentCollectionAsPaid(
      container,
    ).run({
      input: { order_id: mixed.order.id, payment_collection_id: paycol[0].id },
    })
    assert(payment)
    const other = await makeOrder(1)
    const race = await Promise.all([
      call("/admin/devices", {
        action: "assign",
        device_id: a.id,
        order_id: mixed.order.id,
        item_id: mixed.item_id,
      }),
      call("/admin/devices", {
        action: "assign",
        device_id: a.id,
        order_id: other.order.id,
        item_id: other.item_id,
      }),
    ])
    assert.deepEqual(
      race.map((r) => r.status).sort(),
      [200, 400],
      "No double allocation",
    )
    let [assignment] = await service.listDeviceAssignments({
      device_id: a.id,
      status: "reserved",
    })
    if (assignment.order_id !== mixed.order.id) {
      await command({ action: "release", assignment_id: assignment.id })
      await command({
        action: "assign",
        device_id: a.id,
        order_id: mixed.order.id,
        item_id: mixed.item_id,
      })
      ;[assignment] = await service.listDeviceAssignments({
        device_id: a.id,
        status: "reserved",
      })
    }
    const supplierRegistration = await supplierCall("/admin/devices", {
      action: "supplier_assign",
      device: { ...base, source_id: supplier.id, serial_number: `D${stamp}S` },
      order_id: mixed.order.id,
      item_id: mixed.item_id,
    })
    assert.equal(
      supplierRegistration.status,
      200,
      JSON.stringify(supplierRegistration.data),
    )
    const [supplierAssignment] = await service.listDeviceAssignments({
      order_id: mixed.order.id,
      source_id: supplier.id,
    })
    assert.equal(
      await stock(supplier.location_id),
      3,
      "Registering supplier unit does not add stock",
    )
    const mixedListing = await call(`/admin/devices?order_id=${mixed.order.id}`)
    const scopedOrder = await supplierCall(
      `/admin/devices?order_id=${mixed.order.id}`,
    )
    assert.equal(scopedOrder.status, 200)
    assert(!("payment_collections" in scopedOrder.data.order))
    assert.equal(scopedOrder.data.order.shipping_address.address_1, "Test street 1")
    assert(
      scopedOrder.data.assignments.every(
        (a: any) => a.source_id === supplier.id,
      ),
    )
    assert.equal(
      (await supplierCall(`/admin/devices?order_id=${other.order.id}`)).status,
      403,
    )
    assert.equal(mixedListing.status, 200, JSON.stringify(mixedListing.data))
    assert(
      mixedListing.data.devices.some((d: any) => d.id === b.id),
      "Unallocated devices available in order workbench",
    )
    const firstFulfill = await command({
      action: "fulfill",
      assignment_ids: [assignment.id],
    })
    assert.equal(await stock(self.location_id), 2)
    await command({
      action: "cancel_fulfillment",
      fulfillment_id: firstFulfill.result.fulfillment_id,
    })
    assert.equal(
      await stock(self.location_id),
      3,
      "Cancel restores native inventory",
    )
    const ownFulfill = await command({
      action: "fulfill",
      assignment_ids: [assignment.id],
    })
    assert.equal(
      (
        await supplierCall("/admin/devices", {
          action: "fulfill",
          assignment_ids: [assignment.id, supplierAssignment.id],
        })
      ).status,
      403,
    )
    const supplierFulfillResponse = await supplierCall("/admin/devices", {
      action: "fulfill",
      assignment_ids: [supplierAssignment.id],
    })
    assert.equal(
      supplierFulfillResponse.status,
      200,
      JSON.stringify(supplierFulfillResponse.data),
    )
    const supplierFulfill = supplierFulfillResponse.data
    assert.equal(
      (
        await supplierCall("/admin/devices", {
          action: "ship",
          fulfillment_id: ownFulfill.result.fulfillment_id,
          tracking_number: "DENIED",
        })
      ).status,
      403,
    )
    await command({
      action: "ship",
      fulfillment_id: ownFulfill.result.fulfillment_id,
      tracking_number: "TEST-OWN",
    })
    const supplierShip = await supplierCall("/admin/devices", {
      action: "ship",
      fulfillment_id: supplierFulfill.result.fulfillment_id,
      tracking_number: "TEST-SUPPLIER",
    })
    assert.equal(supplierShip.status, 200, JSON.stringify(supplierShip.data))
    await command(
      {
        action: "ship",
        fulfillment_id: ownFulfill.result.fulfillment_id,
        tracking_number: "TEST-OWN",
      },
      400,
    )
    assert.equal(await stock(self.location_id), 2)
    assert.equal(await stock(supplier.location_id), 2)
    const { data: nativeOrders } = await query.graph({
      entity: "order",
      fields: [
        "items.detail.fulfilled_quantity",
        "items.detail.shipped_quantity",
        "fulfillments.id",
        "fulfillments.shipped_at",
      ],
      filters: { id: mixed.order.id },
    })
    assert.equal(
      Number(nativeOrders[0].items![0]!.detail!.shipped_quantity),
      2,
      "Native split shipment updated",
    )
    assert(
      nativeOrders[0].fulfillments!.filter((f) => f?.shipped_at).length === 2,
    )
    assert.equal(
      (
        await call(
          `/store/orders/${mixed.order.id}/devices`,
          undefined,
          token("other", "customer"),
        )
      ).status,
      404,
    )
    const customerDevices = await call(
      `/store/orders/${mixed.order.id}/devices`,
      undefined,
      auth,
    )
    assert.equal(customerDevices.status, 200)
    assert.equal(customerDevices.data.devices.length, 2)
    assert(
      !("cost_cents" in customerDevices.data.devices[0]),
      "Customer cannot access cost",
    )
    const request = await call(
      `/store/orders/${mixed.order.id}/after-sales`,
      {
        type: "exchange",
        reason: "Device exchange check for test fixture",
        items: [{ item_id: mixed.item_id, quantity: 1, device_ids: [a.id] }],
      },
      auth,
    )
    assert.equal(request.status, 201, JSON.stringify(request.data))
    const request_id = request.data.request.id
    assert.equal(
      (
        await call(`/admin/orders/${mixed.order.id}/after-sales`, {
          request_id,
          status: "completed",
        })
      ).status,
      400,
    )
    await call(`/admin/orders/${mixed.order.id}/after-sales`, {
      request_id,
      status: "processing",
    })
    await command(
      {
        action: "receive",
        assignment_id: assignment.id,
        request_id,
        source_id: self.id,
        scan: serial2,
      },
      400,
    )
    await command({
      action: "receive",
      assignment_id: assignment.id,
      request_id,
      source_id: self.id,
      scan: serial,
    })
    assert.equal(await stock(self.location_id), 2, "Returned unit quarantined")
    await command({
      action: "inspect",
      device_id: a.id,
      result: "pass",
      grade: "A",
      battery_health: 90,
      notes: "Returned device QC passed",
    })
    assert.equal(await stock(self.location_id), 3)
    await command({
      action: "assign",
      device_id: b.id,
      order_id: mixed.order.id,
      item_id: mixed.item_id,
      replacement_of: assignment.id,
    })
    const [replacement] = await service.listDeviceAssignments({
      replacement_of: assignment.id,
    })
    assert.notEqual(
      replacement.item_id,
      mixed.item_id,
      "Exchange gets native replacement line",
    )
    const replacementFulfill = await command({
      action: "fulfill",
      assignment_ids: [replacement.id],
    })
    await command({
      action: "ship",
      fulfillment_id: replacementFulfill.result.fulfillment_id,
      tracking_number: "TEST-REPLACEMENT",
    })
    assert.equal(
      (
        await call(`/admin/orders/${mixed.order.id}/after-sales`, {
          request_id,
          status: "completed",
        })
      ).status,
      200,
    )
    const repair = await call(
      `/store/orders/${mixed.order.id}/after-sales`,
      {
        type: "repair",
        reason: "Repair supplier device test fixture",
        items: [
          {
            item_id: mixed.item_id,
            quantity: 1,
            device_ids: [supplierAssignment.device_id],
          },
        ],
      },
      auth,
    )
    assert.equal(repair.status, 201, JSON.stringify(repair.data))
    const repairId = repair.data.request.id
    await call(`/admin/orders/${mixed.order.id}/after-sales`, {
      request_id: repairId,
      status: "processing",
    })
    await command({
      action: "receive",
      assignment_id: supplierAssignment.id,
      request_id: repairId,
      source_id: self.id,
      scan: `D${stamp}S`,
    })
    const beforeRepair = await stock(self.location_id)
    await command({
      action: "inspect",
      device_id: supplierAssignment.device_id,
      result: "pass",
      grade: "A",
      battery_health: 90,
      notes: "Repair successful",
    })
    assert.equal(
      await stock(self.location_id),
      beforeRepair,
      "Repair does not add saleable stock",
    )
    await command({
      action: "repair_ship",
      assignment_id: supplierAssignment.id,
      tracking_number: "TEST-REPAIR",
    })
    assert.equal(
      (
        await call(`/admin/orders/${mixed.order.id}/after-sales`, {
          request_id: repairId,
          status: "completed",
        })
      ).status,
      200,
    )
    const refundRequest = await call(
      `/store/orders/${mixed.order.id}/after-sales`,
      {
        type: "return",
        reason: "Return after repair for fixture refund verification",
        items: [
          {
            item_id: mixed.item_id,
            quantity: 1,
            device_ids: [supplierAssignment.device_id],
          },
        ],
      },
      auth,
    )
    assert.equal(refundRequest.status, 201, JSON.stringify(refundRequest.data))
    const refundId = refundRequest.data.request.id
    await call(`/admin/orders/${mixed.order.id}/after-sales`, {
      request_id: refundId,
      status: "processing",
    })
    await command({
      action: "receive",
      assignment_id: supplierAssignment.id,
      request_id: refundId,
      source_id: self.id,
      scan: `D${stamp}S`,
    })
    const beforeReturn = await stock(self.location_id)
    await command({
      action: "inspect",
      device_id: supplierAssignment.device_id,
      result: "pass",
      grade: "A",
      battery_health: 90,
      notes: "Supplier return to own warehouse QC passed",
    })
    assert.equal(
      await stock(self.location_id),
      beforeReturn + 1,
      "Supplier return becomes own stock only after QC",
    )
    assert.equal(
      (
        await call(`/admin/orders/${mixed.order.id}/after-sales`, {
          request_id: refundId,
          status: "completed",
        })
      ).status,
      400,
      "Cannot close without actual refund",
    )
    assert.equal(
      (
        await call("/admin/devices/refunds", {
          request_id: refundId,
          payment_id: payment.id,
          amount: 999,
          confirmed: true,
        })
      ).status,
      400,
    )
    const refund = await call("/admin/devices/refunds", {
      request_id: refundId,
      payment_id: payment.id,
      amount: 100,
      confirmed: true,
    })
    assert.equal(refund.status, 200, JSON.stringify(refund.data))
    const duplicateRefund = await call("/admin/devices/refunds", {
      request_id: refundId,
      payment_id: payment.id,
      amount: 100,
      confirmed: true,
    })
    assert.equal(duplicateRefund.status, 200)
    assert(
      duplicateRefund.data.result.existing,
      "Refund retry does not pay twice",
    )
    assert.equal(
      (
        await call(`/admin/orders/${mixed.order.id}/after-sales`, {
          request_id: refundId,
          status: "completed",
        })
      ).status,
      200,
    )
    const resale = await makeOrder(1)
    await command({
      action: "assign",
      device_id: supplierAssignment.device_id,
      order_id: resale.order.id,
      item_id: resale.item_id,
    })
    const [resaleAssignment] = await service.listDeviceAssignments({
      order_id: resale.order.id,
    })
    assert.equal(
      resaleAssignment.source_id,
      self.id,
      "Resale uses current warehouse while preserving original supplier provenance",
    )
    const found = await call(
      `/admin/devices?order_id=${resale.order.id}&q=D${stamp}S`,
    )
    assert.equal(found.data.count, 1, "Order scan searches across inventory")
    assert.equal(found.data.devices[0].id, supplierAssignment.device_id)
    const resaleFulfill = await command({
      action: "fulfill",
      assignment_ids: [resaleAssignment.id],
    })
    assert.equal(
      await stock(self.location_id),
      beforeReturn,
      "Resale consumes own warehouse stock",
    )
    await command({
      action: "ship",
      fulfillment_id: resaleFulfill.result.fulfillment_id,
      tracking_number: "TEST-RESALE",
    })
    assert.equal(
      (
        await call(`/admin/orders/${mixed.order.id}/fulfillments`, {
          items: [{ id: mixed.item_id, quantity: 1 }],
        })
      ).status,
      400,
      "Native bypass guarded",
    )
    assert.equal(
      (await call(`/admin/devices?device_id=${a.id}`)).data.events.some(
        (e: any) => e.action === "receive",
      ),
      true,
    )
    console.log(
      "Device checks passed: intake/import atomicity, duplicates, concurrent allocation, supply counts, native mixed-source fulfillment and cancellation, shipment, customer ownership, quarantine/restock, native replacement exchange, repair return, supplier return to own warehouse, actual system-provider refund and retry protection, history and bypass guards.",
    )
    if (process.env.PLAYWRIGHT_MODULE_PATH) {
      const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
      const browser = await chromium.launch({
        channel: "chrome",
        headless: true,
      })
      try {
        const page = await browser.newPage({
          viewport: { width: 390, height: 844 },
        })
        await page
          .context()
          .addCookies([
            { name: "lng", value: "zhCN", url: "http://127.0.0.1:9001" },
          ])
        await page.route("**/admin/**", async (route: any) => {
          if (
            route.request().resourceType() === "fetch" ||
            route.request().resourceType() === "xhr"
          )
            await route.continue({
              headers: {
                ...route.request().headers(),
                authorization: `Bearer ${admin}`,
              },
            })
          else await route.continue()
        })
        await page.goto(
          `http://127.0.0.1:9001/app/device-scan?order_id=${mixed.order.id}`,
        )
        await page.getByTestId("device-scan-page").waitFor()
        await page.getByText("本次订单设备", { exact: true }).waitFor()
        assert(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
          ),
          "No mobile overflow",
        )
        await page.screenshot({
          path: "../../../logs/device-scan-mobile.png",
          fullPage: true,
        })
        await page.setViewportSize({ width: 1440, height: 1000 })
        await page.screenshot({
          path: "../../../logs/device-scan-desktop.png",
          fullPage: true,
        })
        console.log("Device workbench browser checks passed.")
        const supplierContext = await browser.newContext({
          viewport: { width: 390, height: 844 },
        })
        await supplierContext.addCookies([
          { name: "lng", value: "zhCN", url: "http://127.0.0.1:9001" },
        ])
        const supplierPage = await supplierContext.newPage()
        const browserErrors: string[] = []
        supplierPage.on("pageerror", (error: Error) =>
          browserErrors.push(error.message),
        )
        await supplierPage.goto("http://127.0.0.1:9001/app/device-scan")
        await supplierPage
          .locator('input[name="email"]')
          .fill(supplierUser.email)
        await supplierPage.locator('input[type="password"]').fill(password)
        await supplierPage.locator('button[type="submit"]').click()
        await supplierPage.getByTestId("device-scan-page").waitFor()
        await supplierPage.getByText("供应商账号：", { exact: false }).waitFor()
        for (const restricted of ["intake", "inspection"]) {
          await supplierPage.goto(`http://127.0.0.1:9001/app/device-scan/${restricted}`)
          await supplierPage.waitForURL("**/app/device-scan/lookup")
        }
        await supplierPage.goto("http://127.0.0.1:9001/app/device-scan/sources")
        await supplierPage
          .getByText("供应商未分配设备的可供数量", { exact: true })
          .waitFor()
        assert.equal(
          await supplierPage
            .getByText("新增本仓 / 供应商", { exact: true })
            .count(),
          0,
        )
        assert.equal(
          await supplierPage
            .getByText("供应商账号权限", { exact: true })
            .count(),
          0,
        )
        assert(
          await supplierPage.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
          ),
          "Supplier mobile has no overflow",
        )
        await supplierPage.screenshot({
          path: "../../../logs/device-supplier-zhCN-mobile.png",
          fullPage: true,
        })
        await supplierContext.addCookies([
          { name: "lng", value: "en", url: "http://127.0.0.1:9001" },
        ])
        await supplierPage.reload()
        await supplierPage
          .getByRole("heading", { name: "Supplier inventory", exact: true })
          .waitFor()
        await supplierPage.setViewportSize({ width: 1440, height: 1000 })
        await supplierPage.goto("http://127.0.0.1:9001/app/device-scan/orders")
        await supplierPage
          .getByLabel("Authorized orders")
          .selectOption(mixed.order.id)
        const supplierOrderResponse = supplierPage.waitForResponse(
          (response: any) =>
            response.url().includes(`/admin/devices?`) &&
            response.url().includes(`order_id=${mixed.order.id}`),
        )
        await supplierPage
          .getByRole("button", { name: "Load order", exact: true })
          .click()
        const loadedOrder = await supplierOrderResponse
        assert.equal(loadedOrder.status(), 200)
        assert.equal((await loadedOrder.json()).order.id, mixed.order.id)
        await supplierPage.getByLabel("Order item", { exact: true }).waitFor()
        await supplierPage.getByText("Test street 1", { exact: true }).waitFor()
        await supplierPage
          .getByText(`D${stamp}S`, { exact: false })
          .first()
          .waitFor()
        assert.equal(
          await supplierPage
            .getByRole("link", {
              name: "Open order / Refund actions",
              exact: true,
            })
            .count(),
          0,
        )
        assert.deepEqual(browserErrors, [])
        await supplierPage.screenshot({
          path: "../../../logs/device-supplier-en-desktop.png",
          fullPage: true,
        })
        await supplierContext.close()
        console.log(
          "Supplier real login, scoped mobile/desktop UI and Chinese/English checks passed.",
        )
      } finally {
        await browser.close()
      }
    }
    assert.equal(
      (
        await call("/admin/devices/supplier-accounts", {
          ...accountBody,
          order_ids: [],
        })
      ).status,
      200,
    )
    assert.equal(
      (await supplierCall(`/admin/devices?order_id=${mixed.order.id}`)).status,
      403,
      "Existing session loses revoked order access immediately",
    )
    assert.equal(
      (
        await supplierCall("/admin/devices", {
          action: "ship",
          fulfillment_id: supplierFulfill.result.fulfillment_id,
          tracking_number: "DENIED",
        })
      ).status,
      403,
    )
    console.log("Supplier order revocation checks passed.")
  } finally {
    if (supplierAuthId)
      await container
        .resolve(Modules.AUTH)
        .deleteAuthIdentities([supplierAuthId])
    if (supplierUserId) await users.deleteUsers([supplierUserId])
    const assignments = orderIds.length
      ? await service.listDeviceAssignments({ order_id: orderIds })
      : []
    const fixtureDeviceIds = (await service.listDevices({ variant_id })).map(
      (d) => d.id,
    )
    const events = (await service.listDeviceEvents({}, { take: 10000 })).filter(
      (e) =>
        orderIds.includes(e.order_id || "") ||
        fixtureDeviceIds.includes(e.device_id || "") ||
        e.data.variant_id === variant_id ||
        sourceIds.includes(String(e.data.source_id || "")),
    )
    if (events.length) await service.deleteDeviceEvents(events.map((e) => e.id))
    if (assignments.length)
      await service.deleteDeviceAssignments(assignments.map((a) => a.id))
    const deviceRows = variant_id
      ? await service.listDevices({ variant_id })
      : []
    if (deviceRows.length)
      await service.deleteDevices(deviceRows.map((d) => d.id))
    const supplies = sourceIds.length
      ? await service.listDeviceSupplies({ source_id: sourceIds })
      : []
    if (supplies.length)
      await service.deleteDeviceSupplies(supplies.map((s) => s.id))
    const managed = variant_id
      ? await service.listDeviceVariants({ variant_id })
      : []
    if (managed.length)
      await service.deleteDeviceVariants(managed.map((v) => v.id))
    for (const order_id of orderIds) {
      const requests = await afterSales.listAfterSalesRequests({ order_id })
      if (requests.length)
        await afterSales.deleteAfterSalesRequests(requests.map((r) => r.id))
      const { data } = await query.graph({
        entity: "order",
        fields: ["fulfillments.id", "items.id"],
        filters: { id: order_id },
      })
      const reservations = await inventory.listReservationItems({
        line_item_id: data[0]?.items?.map((i) => i!.id) || [],
      })
      if (reservations.length)
        await inventory.deleteReservationItems(reservations.map((r) => r.id))
      const ids = data[0]?.fulfillments?.map((f) => f!.id) || []
      for (const id of ids) {
        // Reset shipment time only on this script's isolated fixtures for removal.
        await fulfillment.updateFulfillment(id, { shipped_at: null })
        await fulfillment.cancelFulfillment(id)
        await fulfillment.deleteFulfillment(id)
      }
      await orders.deleteOrders(order_id)
    }
    if (productIds.length)
      await deleteProductsWorkflow(container).run({
        input: { ids: productIds },
      })
    if (sourceIds.length) await service.deleteDeviceSources(sourceIds)
    if (locationIds.length)
      await deleteStockLocationsWorkflow(container).run({
        input: { ids: locationIds },
      })
    await customers.deleteCustomers(customer.id)
  }
}
