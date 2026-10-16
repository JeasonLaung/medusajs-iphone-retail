import { Resend, type CreateEmailOptions } from "resend"
import { createStep, StepResponse } from "@medusajs/framework/workflows-sdk"
import MailModuleService from "../../modules/mail/service"
import { MAIL_MODULE } from "../../modules/mail"
import {
  decryptSecret,
  escapeHtml,
  publicMailUrl,
} from "../../modules/mail/security"
import { arrayValue, type MailAttachment } from "../../modules/mail/types"

// One provider request per second; the next minute's run handles transient failures.
const pace = () => new Promise((resolve) => setTimeout(resolve, 1000))
const expired = (at: Date | string | null) =>
  !!at && Date.now() - new Date(at).getTime() > 23 * 3600_000

export async function submitMail(
  resend: Resend,
  id: string,
  payload: CreateEmailOptions
) {
  try {
    const { data, error } = await resend.emails.send(payload, {
      idempotencyKey: `mail/${id}`,
      signal: AbortSignal.timeout(8000),
    })
    if (error || !data)
      return {
        id: null,
        error: error?.name || "provider_error",
        retry:
          !!error &&
          [
            "rate_limit_exceeded",
            "application_error",
            "internal_server_error",
            "unknown_error",
          ].includes(error.name),
      }
    return { id: data.id, error: "", retry: false }
  } catch {
    return { id: null, error: "network_error", retry: true }
  }
}

export const processMailQueueStep = createStep(
  "process-mail-queue",
  async (_: Record<string, never>, { container }) => {
    const service: MailModuleService = container.resolve(MAIL_MODULE)
    let processed = 0
    const messages = await service.listMailMessages(
      { folder: "outbox", status: ["queued", "sending"] },
      { take: 10, order: { created_at: "ASC" } }
    )
    for (const message of messages) {
      const account = await service.retrieveMailAccount(message.account_id)
      if (!account.enabled) continue
      if (expired(message.first_attempt_at)) {
        await service.updateMailMessages({
          id: message.id,
          status: "failed",
          error: "retry_expired",
        })
        continue
      }
      const attempts = message.attempts + 1
      await service.updateMailMessages({
        id: message.id,
        status: "sending",
        attempts,
        first_attempt_at: message.first_attempt_at || new Date(),
      })
      const result = await submitMail(
        new Resend(decryptSecret(account.api_key)),
        message.id,
        {
          from: message.from_address,
          to: arrayValue<string>(message.to),
          cc: arrayValue<string>(message.cc),
          bcc: arrayValue<string>(message.bcc),
          subject: message.subject,
          text: message.text_body || message.html_body.replace(/<[^>]+>/g, " "),
          html: message.html_body || undefined,
          replyTo: account.reply_to || account.email,
          attachments: arrayValue<MailAttachment>(message.attachments)
            .filter((a) => a.content)
            .map((a) => ({ filename: a.filename, content: a.content! })),
          headers: message.in_reply_to
            ? {
                "In-Reply-To": message.in_reply_to,
                References: message.in_reply_to,
              }
            : undefined,
          tags: [{ name: "message_id", value: message.id }],
        }
      )
      if (result.id) {
        await service.updateMailMessages({
          id: message.id,
          provider_id: result.id,
          folder: "sent",
          status: "sent",
          sent_at: new Date(),
          error: "",
        })
      } else {
        await service.updateMailMessages({
          id: message.id,
          status: result.retry && attempts < 3 ? "queued" : "failed",
          error: result.error,
        })
      }
      processed++
      await pace()
    }
    const campaigns = await service.listMailCampaigns(
      { status: ["queued", "sending"] },
      { take: 10, order: { updated_at: "ASC" } }
    )
    for (const campaign of campaigns) {
      if (processed >= 10) break
      const account = await service.retrieveMailAccount(campaign.account_id)
      if (!account.enabled) continue
      const resend = new Resend(decryptSecret(account.api_key))
      await service.updateMailCampaigns({ id: campaign.id, status: "sending" })
      const deliveries = await service.listMailDeliveries(
        { campaign_id: campaign.id, status: ["queued", "sending"] },
        { take: 10 - processed, order: { created_at: "ASC" } }
      )
      for (const delivery of deliveries) {
        const contact = await service.retrieveMailContact(delivery.contact_id)
        if (!contact.subscribed || contact.suppressed) {
          await service.updateMailDeliveries({
            id: delivery.id,
            status: "skipped",
            error: "unsubscribed",
          })
          continue
        }
        if (expired(delivery.first_attempt_at)) {
          await service.updateMailDeliveries({
            id: delivery.id,
            status: "failed",
            error: "retry_expired",
          })
          continue
        }
        const url = publicMailUrl(
          `/mail/unsubscribe/${contact.unsubscribe_token}`
        )
        const attempts = delivery.attempts + 1
        await service.updateMailDeliveries({
          id: delivery.id,
          status: "sending",
          attempts,
          first_attempt_at: delivery.first_attempt_at || new Date(),
        })
        const result = await submitMail(resend, delivery.id, {
          from: `${account.name} <${account.email}>`,
          to: delivery.email,
          replyTo: account.reply_to || account.email,
          subject: campaign.subject,
          text: `${campaign.text_body || campaign.html_body.replace(/<[^>]+>/g, " ")}\n\nUnsubscribe / 退订: ${url}`,
          html: `${campaign.html_body || `<div style="white-space:pre-wrap">${escapeHtml(campaign.text_body)}</div>`}<p style="font-size:12px;color:#777"><a href="${escapeHtml(url)}">Unsubscribe / 退订</a></p>`,
          headers: {
            "List-Unsubscribe": `<${url}>`,
            "List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
          },
          tags: [{ name: "delivery_id", value: delivery.id }],
        })
        await service.updateMailDeliveries({
          id: delivery.id,
          ...(result.id
            ? {
                provider_id: result.id,
                status: "sent",
                sent_at: new Date(),
                error: "",
              }
            : {
                status: result.retry && attempts < 3 ? "queued" : "failed",
                error: result.error,
              }),
        })
        processed++
        await pace()
      }
      const [, remaining] = await service.listAndCountMailDeliveries(
        { campaign_id: campaign.id, status: ["queued", "sending"] },
        { take: 1 }
      )
      if (!remaining)
        await service.updateMailCampaigns({
          id: campaign.id,
          status: "completed",
        })
    }
    return new StepResponse({ processed })
  }
)
