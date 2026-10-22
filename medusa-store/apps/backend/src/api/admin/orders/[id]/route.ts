import type { MedusaResponse } from "@medusajs/framework/http"
import { GET as getOrder } from "@medusajs/medusa/api/admin/orders/[id]/route"
import { scopeWarehouseOrderRequest } from "../../../warehouse-guards"

export async function GET(req: Parameters<typeof getOrder>[0], res: MedusaResponse) {
  await scopeWarehouseOrderRequest(req, res, true)
  return getOrder(req, res)
}
