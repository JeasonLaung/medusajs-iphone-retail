import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { WEBHOOK_NOTIFICATIONS_MODULE } from "../../../modules/webhook-notifications"
import type WebhookNotificationsService from "../../../modules/webhook-notifications/service"
import { webhookNotificationsWorkflow, serializeEndpoint } from "../../../workflows/webhook-notifications"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const service: WebhookNotificationsService = req.scope.resolve(WEBHOOK_NOTIFICATIONS_MODULE)
  const endpoints = await service.listWebhookEndpoints({}, { order: { created_at: "DESC" } })
  res.json({ endpoints: endpoints.map(serializeEndpoint) })
}
export async function POST(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const { result } = await webhookNotificationsWorkflow(req.scope).run({ input: { action: "save", data: req.body } })
  res.json({ endpoint: result })
}
