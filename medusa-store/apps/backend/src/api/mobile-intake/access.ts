import type { MedusaContainer } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"
import { verifyIntakeToken } from "../../lib/mobile-intake-token"
import { warehouseAccess, assertWarehouseSource, assertWarehousePermission } from "../../lib/warehouse-access"
import { deviceAccess } from "../device-access"
import { DEVICES_MODULE } from "../../modules/devices"
import type DevicesModuleService from "../../modules/devices/service"

export function intakeSecret(container: MedusaContainer): string {
  const secret = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE).projectConfig.http.jwtSecret
  if (typeof secret !== "string" || !secret)
    throw new MedusaError(MedusaError.Types.UNAUTHORIZED, "MOBILE_INTAKE_LINK_INVALID")
  return secret
}

export async function intakeSource(container: MedusaContainer, actorId: string, sourceId: string) {
  const access = await deviceAccess(container, actorId)
  if (access.role !== "admin") throw new MedusaError(MedusaError.Types.FORBIDDEN, "MOBILE_INTAKE_LINK_INVALID")
  const warehouse = await warehouseAccess(container, actorId)
  if (warehouse.warehouse) await assertWarehousePermission(container, actorId, "warehouse_intake", "update")
  await assertWarehouseSource(container, warehouse, sourceId)
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const source = await service.retrieveDeviceSource(sourceId)
  if (source.type !== "self") throw new MedusaError(MedusaError.Types.INVALID_DATA, "MOBILE_INTAKE_SOURCE_INVALID")
  return source
}

export async function mobileIntakeAccess(container: MedusaContainer, token: string) {
  const session = verifyIntakeToken(token, intakeSecret(container))
  const source = await intakeSource(container, session.actor_id, session.source_id)
  return { session, source }
}
