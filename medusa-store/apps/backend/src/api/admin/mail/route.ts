import type {
  AuthenticatedMedusaRequest,
  MedusaResponse,
} from "@medusajs/framework/http"
import type { FilterQuery } from "@medusajs/framework/types"
import MailModuleService from "../../../modules/mail/service"
import { MAIL_MODULE } from "../../../modules/mail"
import { publicAccount } from "../../../modules/mail/security"
import type { MailListQueryBody } from "./validators"

export async function GET(
  req: AuthenticatedMedusaRequest,
  res: MedusaResponse
) {
  const service: MailModuleService = req.scope.resolve(MAIL_MODULE)
  const query = req.validatedQuery as MailListQueryBody
  const { resource, limit, offset, q, account_id } = query
  const config = {
    take: limit,
    skip: offset,
    order: { created_at: "DESC" as const },
  }
  const search = q ? { $ilike: `%${q.replace(/[%_\\]/g, "\\$&")}%` } : null
  const accountFilter = account_id ? { account_id } : {}
  if (resource === "accounts") {
    const accounts = await service.listMailAccounts(
      {},
      { take: 100, order: { created_at: "ASC" } }
    )
    const base = (
      process.env.MAIL_PUBLIC_URL ||
      process.env.MEDUSA_BACKEND_URL ||
      `${req.protocol}://${req.get("host")}`
    ).replace(/\/+$/, "")
    return res.json({
      webhook_base_url: base,
      accounts: accounts.map((account) => ({
        ...publicAccount(account),
        webhook_url: `${base}/mail/webhooks/${account.id}`,
      })),
    })
  }
  if (resource === "summary") {
    const folders = [
      "inbox",
      "draft",
      "outbox",
      "sent",
      "archive",
      "trash",
    ] as const
    const counts = await Promise.all(
      folders.map(async (folder) => {
        const [, count] = await service.listAndCountMailMessages(
          { ...accountFilter, folder },
          { take: 1 }
        )
        return [folder, count]
      })
    )
    const [, unread] = await service.listAndCountMailMessages(
      { ...accountFilter, folder: "inbox", read: false },
      { take: 1 }
    )
    return res.json({ counts: Object.fromEntries(counts), unread })
  }
  if (resource === "messages") {
    const filters = {
      ...accountFilter,
      folder: query.folder,
      ...(query.unread ? { read: query.unread !== "true" } : {}),
      ...(query.starred ? { starred: query.starred === "true" } : {}),
      ...(search
        ? {
            $or: [
              { subject: search },
              { from_address: search },
              { text_body: search },
            ],
          }
        : {}),
    }
    const [messages, count] = await service.listAndCountMailMessages(
      filters as FilterQuery<unknown>,
      {
        ...config,
        select: [
          "id",
          "account_id",
          "folder",
          "direction",
          "from_address",
          "to",
          "subject",
          "read",
          "starred",
          "status",
          "error",
          "created_at",
          "sent_at",
          "received_at",
        ],
      }
    )
    return res.json({ messages, count, offset, limit })
  }
  if (resource === "contacts") {
    const [contacts, count] = await service.listAndCountMailContacts(
      search
        ? ({
            $or: [{ email: search }, { name: search }],
          } as FilterQuery<unknown>)
        : {},
      config
    )
    // Tokens are only exposed to recipients through their own unsubscribe links.
    return res.json({
      contacts: contacts.map(({ unsubscribe_token: _, ...contact }) => contact),
      count,
      offset,
      limit,
    })
  }
  if (resource === "campaigns") {
    const [campaigns, count] = await service.listAndCountMailCampaigns(
      {
        ...accountFilter,
        ...(search ? { name: search } : {}),
      } as FilterQuery<unknown>,
      config
    )
    const rows = await Promise.all(
      campaigns.map(async (campaign) => {
        const statuses = [
          "queued",
          "sending",
          "sent",
          "delivered",
          "failed",
          "bounced",
          "complained",
          "suppressed",
          "skipped",
        ]
        const totals = await Promise.all(
          statuses.map(async (status) => {
            const [, n] = await service.listAndCountMailDeliveries(
              { campaign_id: campaign.id, status },
              { take: 1 }
            )
            return [status, n]
          })
        )
        const [, opened] = await service.listAndCountMailDeliveries(
          { campaign_id: campaign.id, opened_at: { $ne: null } },
          { take: 1 }
        )
        const [, clicked] = await service.listAndCountMailDeliveries(
          { campaign_id: campaign.id, clicked_at: { $ne: null } },
          { take: 1 }
        )
        return {
          ...campaign,
          stats: { ...Object.fromEntries(totals), opened, clicked },
        }
      })
    )
    return res.json({ campaigns: rows, count, offset, limit })
  }
  if (resource === "deliveries") {
    const [deliveries, count] = await service.listAndCountMailDeliveries(
      query.campaign_id ? { campaign_id: query.campaign_id } : {},
      config
    )
    return res.json({ deliveries, count, offset, limit })
  }
  if (resource === "templates") {
    const [templates, count] = await service.listAndCountMailTemplates(
      search ? ({ name: search } as FilterQuery<unknown>) : {},
      config
    )
    return res.json({ templates, count, offset, limit })
  }
  const [events, count] = await service.listAndCountMailEvents(
    accountFilter,
    config
  )
  res.json({ events, count, offset, limit })
}
