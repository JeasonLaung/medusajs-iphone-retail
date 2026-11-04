import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { acquireLockStep, releaseLockStep } from "@medusajs/medusa/core-flows"
import { MedusaError } from "@medusajs/framework/utils"
import MailModuleService from "../modules/mail/service"
import { MAIL_MODULE } from "../modules/mail"
import { arrayValue } from "../modules/mail/types"
import { fetchRelaymeInbound } from "../modules/mail/relayme-imap"
import { relaymeDeliveryStatus, type RelaymeEventBody, type RelaymeDelivery } from "../modules/mail/relayme"

type Input = { account_ids: string[]; key_id: string; event: RelaymeEventBody }
export async function applyRelaymeEvent(input: Input, service: MailModuleService) {
  const event = input.event
  const eventId = `relayme:${input.key_id}:${event.event_id}`
  const [seen] = await service.listMailEvents({ event_id: eventId }, { take: 1 })
  if (seen) return { ok: true }
  const address = ("username" in event ? event.username : event.mail_from).toLowerCase()
  const accounts = await service.listMailAccounts({ id: input.account_ids, provider: "relayme", enabled: true }, { take: 1000 })
  const account = accounts.find((a) => a.email.toLowerCase() === address)
  // Shared domain callbacks can include mailboxes not configured here.
  if (!account) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "mail.relaymeMailboxMismatch")
  let providerId: string
  if ("username" in event) {
    const message = await fetchRelaymeInbound(account, event)
    providerId = message.provider_id
    const [existing] = await service.listMailMessages({ account_id: account.id, provider_id: providerId }, { take: 1 })
    if (!existing) await service.createMailMessages({ ...message,
      account_id: account.id, folder: "inbox", direction: "inbound", read: false,
      status: "received", received_at: new Date(event.occurred_at),
    })
  } else {
    providerId = event.submission_id
    const [message] = await service.listMailMessages({ account_id: account.id, provider_id: providerId, direction: "outbound" }, { take: 1 })
    const deliveries = await service.listMailDeliveries({ provider_id: providerId }, { take: 100 })
    const history = await service.listMailEvents({ account_id: account.id, provider_id: providerId, type: "relayme.delivery_status" }, { take: 1000 })
    const events = [...history.map((e) => e.data as unknown as RelaymeDelivery), event]
    let matched = false
    if (message) {
      const recipients = [...arrayValue<string>(message.to), ...arrayValue<string>(message.cc), ...arrayValue<string>(message.bcc)]
      if (recipients.some((r) => r.toLowerCase() === event.recipient.toLowerCase())) {
        matched = true
        await service.updateMailMessages({ id: message.id, status: relaymeDeliveryStatus(recipients, events), message_id: event.message_id,
          ...(event.status === "failed" ? { error: "relayme_delivery_failed" } : {}) })
      }
    }
    for (const delivery of deliveries) {
      const campaign = await service.retrieveMailCampaign(delivery.campaign_id)
      if (campaign.account_id !== account.id || delivery.email.toLowerCase() !== event.recipient.toLowerCase()) continue
      matched = true
      await service.updateMailDeliveries({ id: delivery.id, status: relaymeDeliveryStatus([delivery.email], events),
        ...(event.status === "failed" ? { error: "relayme_delivery_failed" } : {}) })
    }
    // Never acknowledge an unassociated result: the provider may have raced a local persistence failure.
    if (!matched) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, "mail.relaymeSubmissionNotFound")
  }
  // Last operation: a failure before this point remains retryable. Stable IMAP identity avoids duplicate messages.
  await service.createMailEvents({ event_id: eventId, account_id: account.id,
    provider_id: providerId, type: "username" in event ? "relayme.received" : "relayme.delivery_status",
    occurred_at: new Date(event.occurred_at),
    data: "username" in event ? { mailbox: event.mailbox, username: event.username } : { ...event },
  })
  return { ok: true }
}
const relaymeWebhookStep = createStep("relayme-webhook", async (input: Input, { container }) => {
  return new StepResponse(await applyRelaymeEvent(input, container.resolve(MAIL_MODULE)))
})

export const relaymeWebhookWorkflow = createWorkflow("relayme-webhook", function (input: Input) {
  acquireLockStep({ key: "mail-operations", timeout: 30, ttl: 180 })
  const result = relaymeWebhookStep(input)
  releaseLockStep({ key: "mail-operations" })
  return new WorkflowResponse(result)
})
