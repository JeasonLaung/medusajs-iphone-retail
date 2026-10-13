import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../../../../../modules/devices"
import DevicesModuleService from "../../../../../modules/devices/service"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const { data } = await query.graph({ entity: "order", fields: ["id", "customer_id"], filters: { id: req.params.id } })
  if (!data[0] || data[0].customer_id !== req.auth_context.actor_id) throw new MedusaError(MedusaError.Types.NOT_FOUND, "Order not found")
  const service: DevicesModuleService = req.scope.resolve(DEVICES_MODULE)
  const assignments = await service.listDeviceAssignments({ order_id: req.params.id, status: ["shipped", "returned"] }, { take: 1000, order: { created_at: "DESC" } })
  const devices = assignments.length ? await service.listDevices({ id: assignments.map((a) => a.device_id) }, { take: 1000 }) : []
  res.json({ devices: assignments.map((a) => {
    const device = devices.find((d) => d.id === a.device_id)
    return { id: a.device_id, assignment_id: a.id, item_id: a.item_id, serial_number: device?.serial_number, imei: device?.imei, status: a.status, replacement_of: a.replacement_of, tracking_number: a.tracking_number }
  }) })
}
