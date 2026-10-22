import type { MedusaResponse } from "@medusajs/framework/http"
import { GET as getLocation } from "@medusajs/medusa/api/admin/stock-locations/[id]/route"
import { assertWarehouseLocation, warehouseAccess } from "../../../../lib/warehouse-access"

export async function GET(req: Parameters<typeof getLocation>[0], res: MedusaResponse) {
  assertWarehouseLocation(await warehouseAccess(req.scope, req.auth_context.actor_id), req.params.id)
  return getLocation(req, res)
}
