import { Resend, type WebhookEventPayload } from "resend"
import {
  createStep,
  createWorkflow,
  StepResponse,
  WorkflowResponse,
} from "@medusajs/framework/workflows-sdk"
import { acquireLockStep, releaseLockStep } from "@medusajs/medusa/core-flows"
import MailModuleService from "../modules/mail/service"
import { MAIL_MODULE } from "../modules/mail"
import { decryptSecret } from "../modules/mail/security"
import { jsonArray } from "../modules/mail/types"
import { MedusaError } from "@medusajs/framework/utils"

export type MailWebhookInput = {
  account_id: string
  event_id: string
  event: WebhookEventPayload
}
const failureStatuses = new Set([
  "bounced",
  "complained",
  "failed",
  "suppressed",
])
// Provider callbacks may arrive out of order; delivery never moves backwards.
export function deliveryStatus(current: string, incoming: string) {
  if (failureStatuses.has(current)) return current
  if (failureStatuses.has(incoming)) return incoming
  if (incoming === "delivered") return "delivered"
  if (current === "delivered") return current
  if (incoming === "sent") return "sent"
  return current
}

const mailWebhookStep = createStep(
  "mail-webhook",
  async (input: MailWebhookInput, { container }) => {
    const service: MailModuleService = container.resolve(MAIL_MODULE)
    const eventId = `${input.account_id}:${input.event_id}`
    const [existing] = await service.listMailEvents(
      { event_id: eventId },
      { take: 1 }
    )
    if (existing) return new StepResponse({ ok: true })
    const event = input.event
    if (!event.type.startsWith("email.") || !("email_id" in event.data))
      return new StepResponse({ ok: true })
    const providerId = event.data.email_id
    if (event.type === "email.received") {
      const [found] = await service.listMailMessages(
        { provider_id: providerId, account_id: input.account_id },
        { take: 1 }
      )
      if (!found) {
        const account = await service.retrieveMailAccount(input.account_id)
        const { data, error } = await new Resend(
          decryptSecret(account.api_key)
        ).emails.receiving.get(providerId, undefined, {
          signal: AbortSignal.timeout(8000),
        })
        if (error || !data)
          throw new MedusaError(
            MedusaError.Types.UNEXPECTED_STATE,
            "mail.receivingUnavailable"
          )
        const recipients = [
          ...data.to,
          ...(data.cc || []),
          ...(data.received_for || []),
        ].map((a) => (a.match(/<([^>]+)>/)?.[1] || a).toLowerCase())
        // A webhook for this account must never import another mailbox's private mail.
        if (recipients.includes(account.email.toLowerCase())) {
          await service.createMailMessages({
            account_id: account.id,
            folder: "inbox",
            direction: "inbound",
            provider_id: providerId,
            from_address: data.from,
            to: jsonArray(data.to),
            cc: jsonArray(data.cc || []),
            bcc: jsonArray([]),
            reply_to: jsonArray(data.reply_to || []),
            subject: data.subject || "",
            text_body: data.text || "",
            html_body: data.html || "",
            message_id: data.message_id,
            in_reply_to: data.headers?.["in-reply-to"] || null,
            attachments: jsonArray(data.attachments || []),
            read: false,
            status: "received",
            received_at: new Date(data.created_at),
          })
        }
      }
    } else {
      const status = event.type.replace("email.", "")
      const occurredAt = new Date(event.created_at)
      const messages = await service.listMailMessages(
        { provider_id: providerId, account_id: input.account_id },
        { take: 1 }
      )
      for (const message of messages)
        await service.updateMailMessages({
          id: message.id,
          status: deliveryStatus(message.status, status),
          ...(failureStatuses.has(status) ? { error: status } : {}),
        })
      const deliveries = await service.listMailDeliveries(
        { provider_id: providerId },
        { take: 1 }
      )
      for (const delivery of deliveries) {
        const campaign = await service.retrieveMailCampaign(
          delivery.campaign_id
        )
        if (campaign.account_id !== input.account_id) continue
        await service.updateMailDeliveries({
          id: delivery.id,
          status: deliveryStatus(delivery.status, status),
          ...(status === "opened" && !delivery.opened_at
            ? { opened_at: occurredAt }
            : {}),
          ...(status === "clicked" && !delivery.clicked_at
            ? { clicked_at: occurredAt }
            : {}),
          ...(failureStatuses.has(status) ? { error: status } : {}),
        })
        if (["bounced", "complained", "suppressed"].includes(status))
          await service.updateMailContacts({
            id: delivery.contact_id,
            subscribed: false,
            suppressed: true,
          })
      }
    }
    // Record only after successful processing, allowing Resend to retry failures.
    await service.createMailEvents({
      event_id: eventId,
      account_id: input.account_id,
      provider_id: providerId,
      type: event.type,
      occurred_at: new Date(event.created_at),
    })
    return new StepResponse({ ok: true })
  }
)

export const mailWebhookWorkflow = createWorkflow(
  "mail-webhook",
  function (input: MailWebhookInput) {
    acquireLockStep({ key: "mail-operations", timeout: 30, ttl: 180 })
    const result = mailWebhookStep(input)
    releaseLockStep({ key: "mail-operations" })
    return new WorkflowResponse(result)
  }
)
