import { randomBytes } from "node:crypto"
import { createStep, StepResponse } from "@medusajs/framework/workflows-sdk"
import { MedusaError } from "@medusajs/framework/utils"
import MailModuleService from "../../modules/mail/service"
import { MAIL_MODULE } from "../../modules/mail"
import {
  encryptSecret,
  publicAccount,
  publicMailUrl,
} from "../../modules/mail/security"
import type { MailCommandBody } from "../../api/admin/mail/validators"
import { jsonArray } from "../../modules/mail/types"

const fail = (code: string): never => {
  throw new MedusaError(MedusaError.Types.INVALID_DATA, `mail.${code}`)
}

export const mailCommandStep = createStep(
  "mail-command",
  async (
    input: MailCommandBody & { actor_id: string },
    { container }
  ): Promise<StepResponse<Record<string, unknown>>> => {
    const service: MailModuleService = container.resolve(MAIL_MODULE)
    const { action } = input
    if (action === "save_account") {
      const previous = input.id
        ? await service.retrieveMailAccount(input.id)
        : null
      const value = {
        name: input.name,
        email: input.email,
        reply_to: input.reply_to,
        enabled: input.enabled,
        api_key: input.api_key
          ? encryptSecret(input.api_key)
          : previous?.api_key || "",
        webhook_secret: input.webhook_secret
          ? encryptSecret(input.webhook_secret)
          : previous?.webhook_secret || "",
      }
      if (!value.api_key) fail("apiKeyRequired")
      return new StepResponse(
        publicAccount(
          previous
            ? await service.updateMailAccounts({ id: previous.id, ...value })
            : await service.createMailAccounts(value)
        )
      )
    }
    if (action === "delete_account") {
      const [, pending] = await service.listAndCountMailMessages(
        { account_id: input.id, folder: "outbox" },
        { take: 1 }
      )
      const [, campaigns] = await service.listAndCountMailCampaigns(
        { account_id: input.id, status: ["queued", "sending", "paused"] },
        { take: 1 }
      )
      if (pending || campaigns) fail("accountBusy")
      // Disable instead of removing credentials needed by historical attachments and callbacks.
      await service.updateMailAccounts({ id: input.id, enabled: false })
    }
    if (action === "save_draft") {
      const account = await service.retrieveMailAccount(input.account_id)
      if (!account.enabled) fail("accountDisabled")
      if (
        input.id &&
        (await service.retrieveMailMessage(input.id)).folder !== "draft"
      )
        fail("draftOnly")
      const value = {
        account_id: account.id,
        to: jsonArray(input.to),
        cc: jsonArray(input.cc),
        bcc: jsonArray(input.bcc),
        subject: input.subject,
        text_body: input.text_body,
        html_body: input.html_body,
        attachments: jsonArray(input.attachments),
        in_reply_to: input.in_reply_to || null,
        from_address: `${account.name} <${account.email}>`,
      }
      const message = input.id
        ? await service.updateMailMessages({ id: input.id, ...value })
        : await service.createMailMessages({
            ...value,
            created_by: input.actor_id,
          })
      return new StepResponse({ id: message.id })
    }
    if (action === "send") {
      const message = await service.retrieveMailMessage(input.id)
      if (message.folder === "outbox" || message.folder === "sent")
        return new StepResponse({ id: message.id })
      if (message.folder !== "draft") fail("draftOnly")
      const account = await service.retrieveMailAccount(message.account_id)
      if (!account.enabled || !account.api_key) fail("accountDisabled")
      if (
        !(message.to as unknown as string[]).length ||
        !message.subject.trim() ||
        (!message.text_body.trim() && !message.html_body.trim())
      )
        fail("messageIncomplete")
      await service.updateMailMessages({
        id: message.id,
        folder: "outbox",
        status: "queued",
        error: "",
      })
      return new StepResponse({ id: message.id })
    }
    if (action === "messages") {
      const messages = await service.listMailMessages(
        { id: input.ids },
        { take: 100 }
      )
      if (messages.length !== new Set(input.ids).size) fail("messageNotFound")
      for (const message of messages) {
        const op = input.operation
        if (
          ["trash", "archive", "delete"].includes(op) &&
          message.folder === "outbox" &&
          message.status !== "failed"
        )
          fail("messageBusy")
        if (op === "delete") {
          if (message.folder !== "trash") fail("trashOnly")
          await service.softDeleteMailMessages(message.id)
        } else if (op === "restore") {
          if (message.folder !== "trash") fail("trashOnly")
          const folder = message.previous_folder as
            "inbox" | "sent" | "draft" | "archive" | "outbox"
          await service.updateMailMessages({ id: message.id, folder })
        } else if (op === "trash" || op === "archive") {
          if (message.folder === op) continue
          await service.updateMailMessages({
            id: message.id,
            folder: op,
            previous_folder: message.folder,
          })
        } else if (op === "retry") {
          if (message.folder !== "outbox" || message.status !== "failed")
            fail("retryFailedOnly")
          // Resend keeps idempotency keys for 24h. Older ambiguous sends require manual review.
          if (
            message.first_attempt_at &&
            Date.now() - new Date(message.first_attempt_at).getTime() >
              23 * 3600_000
          )
            fail("retryExpired")
          await service.updateMailMessages({
            id: message.id,
            status: "queued",
            attempts: 0,
            error: "",
          })
        } else {
          await service.updateMailMessages({
            id: message.id,
            ...(["read", "unread"].includes(op)
              ? { read: op === "read" }
              : { starred: op === "star" }),
          })
        }
      }
    }
    if (action === "contacts") {
      for (const contact of input.contacts) {
        if (contact.subscribed && !contact.consent_source.trim())
          fail("consentRequired")
      }
      for (const contact of input.contacts) {
        const [existing] = await service.listMailContacts(
          { email: contact.email },
          { take: 1 }
        )
        const consent = contact.subscribed
          ? { consent_source: contact.consent_source, consent_at: new Date() }
          : {}
        if (existing) {
          // Import never silently re-subscribes a contact who already opted out.
          await service.updateMailContacts({
            id: existing.id,
            name: contact.name,
            ...(existing.subscribed ? { subscribed: contact.subscribed } : {}),
          })
        } else {
          await service.createMailContacts({
            ...contact,
            ...consent,
            unsubscribe_token: randomBytes(32).toString("hex"),
          })
        }
      }
    }
    if (action === "unsubscribe_contact")
      await service.updateMailContacts({ id: input.id, subscribed: false })
    if (action === "save_campaign") {
      if (!input.text_body.trim() && !input.html_body.trim())
        fail("messageIncomplete")
      if (
        input.id &&
        (await service.retrieveMailCampaign(input.id)).status !== "draft"
      )
        fail("draftOnly")
      const value = {
        account_id: input.account_id,
        name: input.name,
        subject: input.subject,
        text_body: input.text_body,
        html_body: input.html_body,
      }
      const campaign = input.id
        ? await service.updateMailCampaigns({ id: input.id, ...value })
        : await service.createMailCampaigns({
            ...value,
            created_by: input.actor_id,
          })
      return new StepResponse({ id: campaign.id })
    }
    if (action === "campaign") {
      const campaign = await service.retrieveMailCampaign(input.id)
      const account = await service.retrieveMailAccount(campaign.account_id)
      if (input.operation === "queue") {
        if (campaign.status !== "draft") fail("draftOnly")
        if (!account.enabled || !account.api_key) fail("accountDisabled")
        publicMailUrl("/mail/unsubscribe")
        const filter = {
          subscribed: true,
          suppressed: false,
          ...(input.contact_ids ? { id: input.contact_ids } : {}),
        }
        const [, count] = await service.listAndCountMailContacts(filter, {
          take: 1,
        })
        if (!count) fail("noRecipients")
        // Build the durable recipient snapshot in pages; safe to resume after a partial failure.
        for (let skip = 0; skip < count; skip += 500) {
          const contacts = await service.listMailContacts(filter, {
            take: 500,
            skip,
            order: { id: "ASC" },
          })
          const existing = await service.listMailDeliveries(
            { campaign_id: campaign.id, contact_id: contacts.map((c) => c.id) },
            { take: 500 }
          )
          const have = new Set(existing.map((d) => d.contact_id))
          const rows = contacts
            .filter((c) => !have.has(c.id))
            .map((c) => ({
              campaign_id: campaign.id,
              contact_id: c.id,
              email: c.email,
            }))
          if (rows.length) await service.createMailDeliveries(rows)
        }
        await service.updateMailCampaigns({ id: campaign.id, status: "queued" })
      } else if (input.operation === "pause") {
        if (!["queued", "sending"].includes(campaign.status))
          fail("campaignState")
        await service.updateMailCampaigns({ id: campaign.id, status: "paused" })
      } else if (input.operation === "resume") {
        if (campaign.status !== "paused") fail("campaignState")
        if (!account.enabled) fail("accountDisabled")
        await service.updateMailCampaigns({ id: campaign.id, status: "queued" })
      } else {
        if (!["completed", "paused"].includes(campaign.status))
          fail("campaignState")
        const failures = await service.listMailDeliveries(
          { campaign_id: campaign.id, status: "failed" },
          { take: 10_000 }
        )
        for (const delivery of failures) {
          if (
            delivery.first_attempt_at &&
            Date.now() - new Date(delivery.first_attempt_at).getTime() >
              23 * 3600_000
          )
            continue
          await service.updateMailDeliveries({
            id: delivery.id,
            status: "queued",
            attempts: 0,
            error: "",
          })
        }
        await service.updateMailCampaigns({ id: campaign.id, status: "queued" })
      }
    }
    if (action === "save_template") {
      const value = {
        name: input.name,
        subject: input.subject,
        text_body: input.text_body,
        html_body: input.html_body,
      }
      const template = input.id
        ? await service.updateMailTemplates({ id: input.id, ...value })
        : await service.createMailTemplates(value)
      return new StepResponse({ id: template.id })
    }
    if (action === "delete_template")
      await service.softDeleteMailTemplates(input.id)
    return new StepResponse({ ok: true })
  }
)
