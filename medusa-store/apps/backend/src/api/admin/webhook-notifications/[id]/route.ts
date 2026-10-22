import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { webhookNotificationsWorkflow } from "../../../../workflows/webhook-notifications"
export async function POST(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const { result } = await webhookNotificationsWorkflow(req.scope).run({ input: { action: "save", id: req.params.id, data: req.body } })
  res.json({ endpoint: result })
}
export async function DELETE(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  await webhookNotificationsWorkflow(req.scope).run({ input: { action: "delete", id: req.params.id } })
  res.json({ id: req.params.id, deleted: true })
}
