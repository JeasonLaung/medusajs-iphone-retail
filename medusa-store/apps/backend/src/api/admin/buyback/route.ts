import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { BUYBACK_MODULE } from "../../../modules/buyback"
import type BuybackModuleService from "../../../modules/buyback/service"
import { adminBuybackAccess } from "../../../lib/buyback"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  await adminBuybackAccess(req.scope, req.auth_context.actor_id)
  const service = req.scope.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const q = typeof req.query.q === "string" ? req.query.q.trim().slice(0, 100).replace(/[%_]/g, "") : ""
  const statuses = ["awaiting_photos", "photo_review", "ready_to_ship", "in_transit", "received", "final_offer", "accepted", "paid", "return_requested", "returning", "returned", "canceled"] as const
  const status = statuses.find((status) => status === req.query.status)
  const offset = Math.max(0, Math.min(100000, Math.floor(Number(req.query.offset) || 0)))
  const [requests, count] = await service.listAndCountBuybackRequests({ ...(status ? { status } : {}), ...(q ? { $or: [{ id: { $ilike: `%${q}%` } }, { email: { $ilike: `%${q}%` } }, { contact_name: { $ilike: `%${q}%` } }, { tracking_number: { $ilike: `%${q}%` } }] } : {}) }, { take: 20, skip: offset, order: { created_at: "DESC" } })
  res.json({ requests: requests.map(({ submission_key, customer_id, ...request }) => request), count, offset, limit: 20 })
}
