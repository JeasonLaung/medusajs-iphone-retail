import type { ExecArgs, IUserModuleService } from "@medusajs/framework/types"
import { MedusaError, Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import type DevicesModuleService from "../modules/devices/service"
import { deviceOperationWorkflow } from "../workflows/device-operations"

// Idempotent upgrade: only original, unsold intakes have no inspection requirement.
export default async function activateLegacy({ container }: ExecArgs) {
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const users: IUserModuleService = container.resolve(Modules.USER)
  const actor = (await users.listUsers({})).find((user) => user.metadata?.device_role !== "supplier")
  if (!actor) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "An administrator is required")
  const candidates = await service.listDevices({ category: "sale", state: "inspection" }, { take: null })
  let activated = 0
  for (const device of candidates) {
    const events = await service.listDeviceEvents({ device_id: device.id }, { take: null })
    if (!events.some((event) => event.action === "intake") || events.some((event) => ["receive", "inspect"].includes(event.action))) continue
    if ((await service.listDeviceAssignments({ device_id: device.id, status: ["reserved", "packed", "shipped", "returned"] })).length) continue
    await deviceOperationWorkflow(container).run({ input: { action: "activate", device_id: device.id, actor_id: actor.id } })
    activated++
  }
  console.log(`Activated ${activated} original intakes; saleable stock updated through the device workflow.`)
}
