import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { webhookNotificationsWorkflow } from "../../../../../workflows/webhook-notifications"
export async function POST(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  await webhookNotificationsWorkflow(req.scope).run({ input: { action: "test", id: req.params.id } })
  res.json({ success: true })
}
