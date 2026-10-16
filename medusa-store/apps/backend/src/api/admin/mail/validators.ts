import { z } from "@medusajs/framework/zod"

const id = z.string().min(1).max(255)
const email = z
  .email()
  .max(254)
  .transform((value) => value.toLowerCase())
const attachment = z.object({
  filename: z.string().min(1).max(255),
  content: z
    .string()
    .max(14_000_000)
    .regex(/^[A-Za-z0-9+/]*={0,2}$/),
})
export const MailCommand = z.discriminatedUnion("action", [
  z.object({
    action: z.literal("save_account"),
    id: id.optional(),
    name: z
      .string()
      .min(1)
      .max(255)
      .regex(/^[^\r\n<>]*$/),
    email,
    reply_to: z.union([email, z.literal("")]).default(""),
    enabled: z.boolean(),
    api_key: z.string().max(512).default(""),
    webhook_secret: z.string().max(512).default(""),
  }),
  z.object({ action: z.literal("delete_account"), id }),
  z
    .object({
      action: z.literal("save_draft"),
      id: id.optional(),
      account_id: id,
      to: z.array(email).max(50),
      cc: z.array(email).max(50).default([]),
      bcc: z.array(email).max(50).default([]),
      subject: z
        .string()
        .max(998)
        .regex(/^[^\r\n]*$/),
      text_body: z.string().max(500_000).default(""),
      html_body: z.string().max(1_000_000).default(""),
      attachments: z.array(attachment).max(10).default([]),
      in_reply_to: z
        .string()
        .max(998)
        .regex(/^[^\r\n]*$/)
        .nullable()
        .optional(),
    })
    .refine(
      (value) =>
        value.attachments.reduce((total, a) => total + a.content.length, 0) <=
        14_000_000,
      "mail.attachmentsTooLarge"
    )
    .refine(
      (value) => value.to.length + value.cc.length + value.bcc.length <= 50,
      "mail.invalidRecipients"
    ),
  z.object({ action: z.literal("send"), id }),
  z.object({
    action: z.literal("messages"),
    ids: z.array(id).min(1).max(100),
    operation: z.enum([
      "read",
      "unread",
      "star",
      "unstar",
      "trash",
      "restore",
      "archive",
      "delete",
      "retry",
    ]),
  }),
  z.object({
    action: z.literal("contacts"),
    contacts: z
      .array(
        z.object({
          email,
          name: z.string().max(255).default(""),
          subscribed: z.boolean().default(false),
          consent_source: z.string().max(500).default(""),
        })
      )
      .min(1)
      .max(1000),
  }),
  z.object({ action: z.literal("unsubscribe_contact"), id }),
  z.object({
    action: z.literal("save_campaign"),
    id: id.optional(),
    account_id: id,
    name: z.string().min(1).max(255),
    subject: z
      .string()
      .min(1)
      .max(998)
      .regex(/^[^\r\n]*$/),
    text_body: z.string().max(500_000).default(""),
    html_body: z.string().max(1_000_000).default(""),
  }),
  z.object({
    action: z.literal("campaign"),
    id,
    operation: z.enum(["queue", "pause", "resume", "retry"]),
    contact_ids: z.array(id).max(10_000).optional(),
  }),
  z.object({
    action: z.literal("save_template"),
    id: id.optional(),
    name: z.string().min(1).max(255),
    subject: z.string().max(998),
    text_body: z.string().max(500_000).default(""),
    html_body: z.string().max(1_000_000).default(""),
  }),
  z.object({ action: z.literal("delete_template"), id }),
])
export type MailCommandBody = z.infer<typeof MailCommand>

export const MailListQuery = z.object({
  resource: z
    .enum([
      "messages",
      "summary",
      "accounts",
      "contacts",
      "campaigns",
      "deliveries",
      "templates",
      "events",
    ])
    .default("messages"),
  folder: z
    .enum(["inbox", "draft", "outbox", "sent", "archive", "trash"])
    .default("inbox"),
  account_id: id.optional(),
  campaign_id: id.optional(),
  q: z.string().max(255).default(""),
  unread: z.enum(["true", "false"]).optional(),
  starred: z.enum(["true", "false"]).optional(),
  offset: z.coerce.number().int().min(0).default(0),
  limit: z.coerce.number().int().min(1).max(100).default(30),
})
export type MailListQueryBody = z.infer<typeof MailListQuery>
