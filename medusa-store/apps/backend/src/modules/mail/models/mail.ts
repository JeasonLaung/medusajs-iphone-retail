import { model } from "@medusajs/framework/utils"
import { jsonArray } from "../types"

export const MailAccount = model
  .define("mail_account", {
    id: model.id({ prefix: "macc" }).primaryKey(),
    name: model.text(),
    email: model.text(),
    reply_to: model.text().default(""),
    enabled: model.boolean().default(true),
    api_key: model.text().default(""),
    webhook_secret: model.text().default(""),
  })
  .indexes([{ on: ["email"], unique: true }])

export const MailMessage = model
  .define("mail_message", {
    id: model.id({ prefix: "mail" }).primaryKey(),
    account_id: model.text(),
    folder: model
      .enum(["inbox", "draft", "outbox", "sent", "archive", "trash"])
      .default("draft"),
    previous_folder: model.text().default("inbox"),
    direction: model.enum(["inbound", "outbound"]).default("outbound"),
    from_address: model.text().default(""),
    to: model.json().default(jsonArray([])),
    cc: model.json().default(jsonArray([])),
    bcc: model.json().default(jsonArray([])),
    subject: model.text().default(""),
    text_body: model.text().default(""),
    html_body: model.text().default(""),
    attachments: model.json().default(jsonArray([])),
    provider_id: model.text().nullable(),
    message_id: model.text().nullable(),
    in_reply_to: model.text().nullable(),
    reply_to: model.json().default(jsonArray([])),
    read: model.boolean().default(true),
    starred: model.boolean().default(false),
    status: model.text().default("draft"),
    error: model.text().default(""),
    attempts: model.number().default(0),
    first_attempt_at: model.dateTime().nullable(),
    sent_at: model.dateTime().nullable(),
    received_at: model.dateTime().nullable(),
    created_by: model.text().default(""),
  })
  .indexes([
    { on: ["account_id", "folder"] },
    { on: ["account_id", "provider_id"], unique: true },
    { on: ["folder", "read"] },
  ])

export const MailContact = model
  .define("mail_contact", {
    id: model.id({ prefix: "mcon" }).primaryKey(),
    email: model.text(),
    name: model.text().default(""),
    subscribed: model.boolean().default(false),
    consent_source: model.text().default(""),
    consent_at: model.dateTime().nullable(),
    suppressed: model.boolean().default(false),
    unsubscribe_token: model.text(),
  })
  .indexes([
    { on: ["email"], unique: true },
    { on: ["unsubscribe_token"], unique: true },
  ])

export const MailCampaign = model
  .define("mail_campaign", {
    id: model.id({ prefix: "mcamp" }).primaryKey(),
    name: model.text(),
    account_id: model.text(),
    subject: model.text(),
    text_body: model.text().default(""),
    html_body: model.text().default(""),
    status: model
      .enum(["draft", "queued", "sending", "paused", "completed"])
      .default("draft"),
    created_by: model.text(),
  })
  .indexes([{ on: ["status"] }])

export const MailDelivery = model
  .define("mail_delivery", {
    id: model.id({ prefix: "mdel" }).primaryKey(),
    campaign_id: model.text(),
    contact_id: model.text(),
    email: model.text(),
    status: model.text().default("queued"),
    provider_id: model.text().nullable(),
    attempts: model.number().default(0),
    first_attempt_at: model.dateTime().nullable(),
    error: model.text().default(""),
    sent_at: model.dateTime().nullable(),
    opened_at: model.dateTime().nullable(),
    clicked_at: model.dateTime().nullable(),
  })
  .indexes([
    { on: ["campaign_id", "contact_id"], unique: true },
    { on: ["campaign_id", "status"] },
    { on: ["provider_id"] },
  ])

export const MailEvent = model
  .define("mail_event", {
    id: model.id({ prefix: "mevt" }).primaryKey(),
    event_id: model.text(),
    account_id: model.text(),
    type: model.text(),
    provider_id: model.text(),
    occurred_at: model.dateTime(),
  })
  .indexes([{ on: ["event_id"], unique: true }])

export const MailTemplate = model.define("mail_template", {
  id: model.id({ prefix: "mtpl" }).primaryKey(),
  name: model.text(),
  subject: model.text().default(""),
  text_body: model.text().default(""),
  html_body: model.text().default(""),
})
