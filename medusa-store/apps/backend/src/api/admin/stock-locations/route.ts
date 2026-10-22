import type { MedusaResponse } from "@medusajs/framework/http"
import { GET as listLocations } from "@medusajs/medusa/api/admin/stock-locations/route"
import { warehouseAccess } from "../../../lib/warehouse-access"

export async function GET(req: Parameters<typeof listLocations>[0], res: MedusaResponse) {
  const access = await warehouseAccess(req.scope, req.auth_context.actor_id)
  if (access.location_ids !== null) req.filterableFields = { $and: [req.filterableFields || {}, { id: access.location_ids.length ? access.location_ids : ["__no_warehouse__"] }] }
  return listLocations(req, res)
}
