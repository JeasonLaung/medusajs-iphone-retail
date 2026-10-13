import type {
  ExecArgs,
  ICustomerModuleService,
  IOrderModuleService,
  IInventoryService,
  IFulfillmentModuleService,
  IPaymentModuleService,
} from "@medusajs/framework/types"
import {
  Modules,
  ContainerRegistrationKeys,
  MedusaError,
} from "@medusajs/framework/utils"
import {
  deleteProductsWorkflow,
  deleteStockLocationsWorkflow,
} from "@medusajs/medusa/core-flows"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"
import { AFTER_SALES_MODULE } from "../modules/after-sales"
import AfterSalesModuleService from "../modules/after-sales/service"

// Only artifacts owned by check-devices.ts, identified by its exact timestamp convention.
export default async function cleanDeviceChecks({ container }: ExecArgs) {
  const customers: ICustomerModuleService = container.resolve(Modules.CUSTOMER)
  const orders: IOrderModuleService = container.resolve(Modules.ORDER)
  const inventory: IInventoryService = container.resolve(Modules.INVENTORY)
  const fulfillment: IFulfillmentModuleService = container.resolve(
    Modules.FULFILLMENT,
  )
  const payment: IPaymentModuleService = container.resolve(Modules.PAYMENT)
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const afterSales: AfterSalesModuleService =
    container.resolve(AFTER_SALES_MODULE)
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const sources = await service.listDeviceSources({}, { take: 1000 })
  const fixtureCustomers = (
    await customers.listCustomers(
      { email: { $like: "device-%@example.invalid" } },
      { take: 1000 },
    )
  ).filter((c) => /^device-\d{13}@example\.invalid$/.test(c.email || ""))
  const stamps = new Set(
    fixtureCustomers.map((c) => c.email!.match(/\d{13}/)![0]),
  )
  for (const source of sources) {
    const match = source.name.match(/^Check (?:warehouse|supplier) (\d{13})$/)
    if (match) stamps.add(match[1])
  }
  for (const stamp of stamps) {
    const fixtureEmail = `device-${stamp}@example.invalid`
    const { data: products } = await query.graph({
      entity: "product",
      fields: ["id", "title", "variants.id"],
      filters: { handle: `device-check-${stamp}` },
    })
    if (products.length && products[0].title !== `Device check ${stamp}`)
      throw new MedusaError(
        MedusaError.Types.INVALID_DATA,
        "Fixture identity mismatch",
      )
    const variantIds = products.flatMap(
      (p) => p.variants?.map((v) => v!.id) || [],
    )
    const { data: fixtureOrders } = await query.graph({
      entity: "order",
      fields: [
        "id",
        "email",
        "items.id",
        "items.title",
        "fulfillments.id",
        "payment_collections.id",
      ],
      filters: { email: fixtureEmail },
    })
    for (const order of fixtureOrders) {
      if (
        !order.items?.every(
          (i) =>
            i?.title === "Device check" ||
            i?.title?.startsWith("Device check "),
        )
      )
        throw new MedusaError(
          MedusaError.Types.INVALID_DATA,
          "Fixture order identity mismatch",
        )
      const rows = await afterSales.listAfterSalesRequests({
        order_id: order.id,
      })
      if (rows.length)
        await afterSales.deleteAfterSalesRequests(rows.map((r) => r.id))
      const assignments = await service.listDeviceAssignments({
        order_id: order.id,
      })
      if (assignments.length)
        await service.deleteDeviceAssignments(assignments.map((a) => a.id))
      const events = await service.listDeviceEvents({ order_id: order.id })
      if (events.length)
        await service.deleteDeviceEvents(events.map((e) => e.id))
      const reservations = await inventory.listReservationItems({
        line_item_id: order.items?.map((i) => i!.id) || [],
      })
      if (reservations.length)
        await inventory.deleteReservationItems(reservations.map((r) => r.id))
      for (const f of order.fulfillments || []) {
        if (!f) continue
        await fulfillment.updateFulfillment(f.id, { shipped_at: null })
        await fulfillment.cancelFulfillment(f.id)
        await fulfillment.deleteFulfillment(f.id)
      }
      if (order.payment_collections?.length)
        await payment.deletePaymentCollections(
          order.payment_collections.map((p) => p!.id),
        )
      await orders.deleteOrders(order.id)
    }
    const fixtureSources = sources.filter((s) =>
      [`Check warehouse ${stamp}`, `Check supplier ${stamp}`].includes(s.name),
    )
    const sourceIds = fixtureSources.map((s) => s.id)
    const devices = variantIds.length
      ? await service.listDevices({ variant_id: variantIds })
      : []
    const events = (await service.listDeviceEvents({}, { take: 10000 })).filter(
      (e) =>
        devices.some((d) => d.id === e.device_id) ||
        variantIds.includes(String(e.data.variant_id || "")) ||
        sourceIds.includes(String(e.data.source_id || "")),
    )
    if (events.length) await service.deleteDeviceEvents(events.map((e) => e.id))
    if (devices.length) await service.deleteDevices(devices.map((d) => d.id))
    const supplies = sourceIds.length
      ? await service.listDeviceSupplies({ source_id: sourceIds })
      : []
    if (supplies.length)
      await service.deleteDeviceSupplies(supplies.map((s) => s.id))
    const managed = variantIds.length
      ? await service.listDeviceVariants({ variant_id: variantIds })
      : []
    if (managed.length)
      await service.deleteDeviceVariants(managed.map((v) => v.id))
    if (products.length)
      await deleteProductsWorkflow(container).run({
        input: { ids: products.map((p) => p.id) },
      })
    if (sourceIds.length) await service.deleteDeviceSources(sourceIds)
    if (fixtureSources.length)
      await deleteStockLocationsWorkflow(container).run({
        input: { ids: fixtureSources.map((s) => s.location_id) },
      })
    const customer = fixtureCustomers.find((c) => c.email === fixtureEmail)
    if (customer) await customers.deleteCustomers(customer.id)
  }
  console.log(`Removed isolated device-check fixtures for ${stamps.size} runs.`)
}
