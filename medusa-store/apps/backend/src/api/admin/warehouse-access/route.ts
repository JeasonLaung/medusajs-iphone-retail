import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import type { z } from "@medusajs/framework/zod"
import { WarehouseBindingBody } from "./validators"
import { warehouseAccess } from "../../../lib/warehouse-access"
import { Modules } from "@medusajs/framework/utils"
import { assertWarehouseAdministrator } from "../../../lib/warehouse-role"
import { updateWarehouseAccessWorkflow } from "../../../workflows/warehouse-access"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  await assertWarehouseAdministrator(req.scope, req.auth_context.actor_id)
  const users = await req.scope.resolve(Modules.USER).listUsers({}, { take: null })
  const locations = await req.scope.resolve(Modules.STOCK_LOCATION).listStockLocations({}, { take: null })
  const visibleUsers = await Promise.all(users.filter((user) => user.metadata?.device_role !== "supplier").map(async (user) => {
    const access = await warehouseAccess(req.scope, user.id)
    return { id: user.id, email: user.email, scope: !access.warehouse ? "platform" : access.location_ids === null ? "all" : "bound", location_ids: access.location_ids || [] }
  }))
  res.json({ actor_id: req.auth_context.actor_id, locations: locations.map(({ id, name }) => ({ id, name })), users: visibleUsers })
}
export async function POST(req: AuthenticatedMedusaRequest<z.infer<typeof WarehouseBindingBody>>, res: MedusaResponse) {
  const { result } = await updateWarehouseAccessWorkflow(req.scope).run({ input: { ...req.validatedBody, actor_id: req.auth_context.actor_id } })
  res.json(result)
}
