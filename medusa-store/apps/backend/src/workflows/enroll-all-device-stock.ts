import type { IInventoryService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, generateEntityId, MedusaError, Modules } from "@medusajs/framework/utils"
import { createStep, createWorkflow, StepResponse, WorkflowResponse, when } from "@medusajs/framework/workflows-sdk"
import { acquireLockStep, releaseLockStep, updateInventoryLevelsWorkflow } from "@medusajs/medusa/core-flows"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"

type Input = { actor_id: string; preserve_reservations: boolean }

const prepareAllDeviceStock = createStep("prepare-all-device-stock", async (input: Input, { container }) => {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const inventory: IInventoryService = container.resolve(Modules.INVENTORY)
  const { data: variants } = await query.graph({ entity: "product_variant", fields: ["id", "sku", "manage_inventory", "allow_backorder", "inventory_items.inventory_item_id", "inventory_items.required_quantity"], pagination: { take: 100000 } })
  const managed = await service.listDeviceVariants({}, { take: null })
  const pending = variants.filter(v => !managed.some(m => m.variant_id === v.id))
  const used = new Set<string>()
  for (const v of variants) {
    const id = v.inventory_items?.[0]?.inventory_item_id
    if (!v.manage_inventory || v.allow_backorder || v.inventory_items?.length !== 1 || Number(v.inventory_items[0]?.required_quantity) !== 1 || !id || used.has(id)) {
      throw new MedusaError(MedusaError.Types.INVALID_DATA, `SKU ${v.sku}: requires dedicated unit inventory, tracking enabled and backorders disabled`)
    }
    used.add(id)
  }
  const devices = await service.listDevices({ state: ["available", "reserved"] }, { take: null })
  const supplies = await service.listDeviceSupplies({}, { take: null })
  const sources = await service.listDeviceSources({}, { take: null })
  const levels = await inventory.listInventoryLevels({}, { take: null })
  const quantities = new Map<string, number>()
  const add = (variant: string, location: string, quantity: number) => {
    const key = `${variant}:${location}`
    quantities.set(key, (quantities.get(key) || 0) + quantity)
  }
  for (const d of devices) add(d.variant_id, d.location_id, 1)
  for (const supply of supplies) {
    const source = sources.find(s => s.id === supply.source_id)
    if (!source) throw new MedusaError(MedusaError.Types.INVALID_DATA, `Missing source for ${supply.id}`)
    add(supply.variant_id, source.location_id, Number(supply.quantity))
  }
  const rows = pending.map(v => ({ id: generateEntityId(undefined, "dvar"), variant_id: v.id, inventory_item_id: v.inventory_items![0]!.inventory_item_id! }))
  const updates = rows.flatMap(v => levels.filter(l => l.inventory_item_id === v.inventory_item_id).map(l => {
    const actual = quantities.get(`${v.variant_id}:${l.location_id}`) || 0
    const reserved = Number(l.reserved_quantity)
    if (reserved > actual && !input.preserve_reservations) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, `Existing reservation requires review: ${v.variant_id}`)
    return { id: l.id, inventory_item_id: l.inventory_item_id, location_id: l.location_id, stocked_quantity: Math.max(actual, reserved) }
  }))
  // A missing level with tracked stock is inconsistent and must be repaired first.
  for (const v of rows) for (const [key, quantity] of quantities) {
    if (quantity > 0 && key.startsWith(`${v.variant_id}:`) && !updates.some(l => `${v.variant_id}:${l.location_id}` === key)) throw new MedusaError(MedusaError.Types.INVALID_DATA, `Tracked stock has no level: ${key}`)
  }
  const previous = levels.filter(l => rows.some(v => v.inventory_item_id === l.inventory_item_id))
  return new StepResponse({ rows, updates, actor_id: input.actor_id, previous, preserve_reservations: input.preserve_reservations })
})

const registerAllDeviceStock = createStep("register-all-device-stock", async (plan: { rows: { id: string; variant_id: string; inventory_item_id: string }[]; actor_id: string; previous: unknown[]; preserve_reservations: boolean }, { container }) => {
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const ids: string[] = []
  const eventId = generateEntityId(undefined, "devt")
  try {
    for (let offset = 0; offset < plan.rows.length; offset += 200) {
      const batch = plan.rows.slice(offset, offset + 200)
      await service.createDeviceVariants(batch)
      ids.push(...batch.map(r => r.id))
    }
    if (ids.length) await service.createDeviceEvents({ id: eventId, actor_id: plan.actor_id, action: "enroll_all", data: { variants: plan.rows, previous_levels: plan.previous, preserve_reservations: plan.preserve_reservations } })
  } catch (error) {
    if (ids.length) await service.deleteDeviceVariants(ids)
    throw error
  }
  return new StepResponse({ enrolled: ids.length }, { ids, eventId })
}, async (previous, { container }) => {
  if (!previous?.ids.length) return
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  await service.deleteDeviceEvents(previous.eventId)
  await service.deleteDeviceVariants(previous.ids)
})

export const enrollAllDeviceStockWorkflow = createWorkflow("enroll-all-device-stock", function (input: Input) {
  acquireLockStep({ key: "device-operations", timeout: 30, ttl: 180 })
  const plan = prepareAllDeviceStock(input)
  when(plan, p => p.updates.length > 0).then(() => {
    updateInventoryLevelsWorkflow.runAsStep({ input: { updates: plan.updates } })
  })
  const result = registerAllDeviceStock(plan)
  releaseLockStep({ key: "device-operations" })
  return new WorkflowResponse(result)
})
