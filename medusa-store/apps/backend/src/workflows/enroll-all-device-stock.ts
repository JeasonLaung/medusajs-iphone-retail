import { ContainerRegistrationKeys, generateEntityId, MedusaError } from "@medusajs/framework/utils"
import { createStep, createWorkflow, StepResponse, WorkflowResponse, when } from "@medusajs/framework/workflows-sdk"
import { acquireLockStep, releaseLockStep, updateInventoryLevelsWorkflow } from "@medusajs/medusa/core-flows"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"

type Input = { actor_id: string; preserve_reservations: boolean }

const prepareAllDeviceStock = createStep("prepare-all-device-stock", async (input: Input, { container }) => {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
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
  const rows = pending.map(v => ({ id: generateEntityId(undefined, "dvar"), variant_id: v.id, inventory_item_id: v.inventory_items![0]!.inventory_item_id! }))
  // Enrolment enables serial tracking while preserving stock and reservations.
  const updates: { id: string; inventory_item_id: string; location_id: string; stocked_quantity: number }[] = []
  const previous: unknown[] = []
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
