import { Resend, type WebhookEventPayload } from "resend"
import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import MailModuleService from "../../../../modules/mail/service"
import { MAIL_MODULE } from "../../../../modules/mail"
import { decryptSecret } from "../../../../modules/mail/security"
import { mailWebhookWorkflow } from "../../../../workflows/mail-webhook"

export async function POST(req: MedusaRequest, res: MedusaResponse) {
  const service: MailModuleService = req.scope.resolve(MAIL_MODULE)
  const [account] = await service.listMailAccounts(
    { id: req.params.account_id },
    { take: 1 }
  )
  if (!account?.webhook_secret) return res.sendStatus(401)
  let event: WebhookEventPayload
  const eventId = req.get("svix-id") || ""
  try {
    event = new Resend(decryptSecret(account.api_key)).webhooks.verify({
      payload: req.rawBody?.toString("utf8") || "",
      headers: {
        id: eventId,
        timestamp: req.get("svix-timestamp") || "",
        signature: req.get("svix-signature") || "",
      },
      webhookSecret: decryptSecret(account.webhook_secret),
    })
  } catch {
    return res.sendStatus(401)
  }
  await mailWebhookWorkflow(req.scope).run({
    input: { account_id: account.id, event_id: eventId, event },
  })
  res.sendStatus(204)
}
