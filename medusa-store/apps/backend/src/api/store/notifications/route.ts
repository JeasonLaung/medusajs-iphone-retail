import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { CUSTOMER_NOTIFICATIONS_MODULE } from "../../../modules/customer-notifications"
import type CustomerNotificationsService from "../../../modules/customer-notifications/service"
import { readCustomerNoticesWorkflow } from "../../../workflows/customer-notifications"
import type { ReadNoticesBody } from "./validators"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const service = req.scope.resolve<CustomerNotificationsService>(CUSTOMER_NOTIFICATIONS_MODULE)
  const customer_id = req.auth_context.actor_id
  const offset = Math.max(0, Math.min(100000, Math.floor(Number(req.query.offset) || 0)))
  const [[notifications, count], [, unread]] = await Promise.all([
    service.listAndCountCustomerNotices({ customer_id }, { take: 20, skip: offset, order: { created_at: "DESC", id: "DESC" } }),
    service.listAndCountCustomerNotices({ customer_id, read_at: null }, { take: 0 }),
  ])
  res.json({ notifications, count, unread, offset, limit: 20 })
}
export async function POST(req: AuthenticatedMedusaRequest<ReadNoticesBody>, res: MedusaResponse) {
  const { result } = await readCustomerNoticesWorkflow(req.scope).run({ input: { customer_id: req.auth_context.actor_id, id: req.validatedBody.id } })
  res.json(result)
}
