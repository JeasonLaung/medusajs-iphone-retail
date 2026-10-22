import type { MedusaResponse } from "@medusajs/framework/http"
import { GET as listOrders } from "@medusajs/medusa/api/admin/orders/route"
import { scopeWarehouseOrderRequest } from "../../warehouse-guards"

export async function GET(req: Parameters<typeof listOrders>[0], res: MedusaResponse) {
  await scopeWarehouseOrderRequest(req, res)
  return listOrders(req, res)
}
