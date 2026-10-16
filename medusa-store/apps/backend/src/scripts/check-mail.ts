import assert from "node:assert/strict"
import { createHmac, randomBytes } from "node:crypto"
import { mkdir } from "node:fs/promises"
import { join } from "node:path"
import type {
  ConfigModule,
  ExecArgs,
  IUserModuleService,
} from "@medusajs/framework/types"
import {
  ContainerRegistrationKeys,
  generateJwtToken,
  Modules,
} from "@medusajs/framework/utils"
import MailModuleService from "../modules/mail/service"
import { MAIL_MODULE } from "../modules/mail"
import { encryptSecret } from "../modules/mail/security"
import { jsonArray } from "../modules/mail/types"
import { mailCommandWorkflow } from "../workflows/mail-command"
import { mailWebhookWorkflow } from "../workflows/mail-webhook"
import { processMailQueueWorkflow } from "../workflows/process-mail-queue"

// Isolated fixtures + mocked Resend. Never uses real recipient addresses or provider credentials.
// Optional browser QA: MAIL_PLAYWRIGHT_MODULE and MAIL_QA_DIR point to local test dependencies/output.
export default async function checkMail({ container }: ExecArgs) {
  const service: MailModuleService = container.resolve(MAIL_MODULE)
  const users: IUserModuleService = container.resolve(Modules.USER)
  const config: ConfigModule = container.resolve(
    ContainerRegistrationKeys.CONFIG_MODULE
  )
  const [user] = await users.listUsers({})
  assert(user, "An admin user is required")
  const token = generateJwtToken(
    {
      actor_id: user.id,
      actor_type: "user",
      auth_identity_id: "mail_smoke_test",
      app_metadata: { user_id: user.id },
    },
    { secret: config.projectConfig.http.jwtSecret, expiresIn: "10m" }
  )
  const base = process.env.MAIL_TEST_URL || "http://127.0.0.1:9002"
  const headers = {
    Authorization: `Bearer ${token}`,
    "Content-Type": "application/json",
  }
  const originalFetch = globalThis.fetch
  const prefix = `qa-${Date.now()}`
  const secret = `whsec_${randomBytes(32).toString("base64")}`
  const originalPublicUrl = process.env.MAIL_PUBLIC_URL
  process.env.MAIL_PUBLIC_URL = base
  const account = await service.createMailAccounts({
    name: "Eurorefurb Support",
    email: `${prefix}@example.invalid`,
    api_key: encryptSecret("re_mail_qa_invalid"),
    webhook_secret: encryptSecret(secret),
  })
  const command = async (body: Record<string, unknown>) => {
    const response = await originalFetch(`${base}/admin/mail/commands`, {
      method: "POST",
      headers,
      body: JSON.stringify(body),
    })
    const data = (await response.json()) as Record<string, unknown>
    assert.equal(response.status, 200, JSON.stringify(data))
    return data
  }
  const get = async (resource: string, query = "") => {
    const response = await originalFetch(
      `${base}/admin/mail?resource=${resource}${query}`,
      { headers }
    )
    assert.equal(response.status, 200, await response.clone().text())
    return response.json() as Promise<Record<string, any>>
  }
  let browser: any
  try {
    assert.equal(
      (await originalFetch(`${base}/admin/mail?resource=summary`)).status,
      401
    )
    assert.equal(
      (
        await originalFetch(`${base}/admin/mail/commands`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ action: "delete_account", id: account.id }),
        })
      ).status,
      401
    )
    const safeAccounts = await get("accounts")
    const safe = safeAccounts.accounts.find(
      (row: { id: string }) => row.id === account.id
    )
    assert.equal(safe.has_api_key, true)
    assert.equal(safe.api_key, undefined)
    assert.equal(safe.webhook_secret, undefined)
    const draft = await command({
      action: "save_draft",
      account_id: account.id,
      to: ["customer@example.invalid"],
      cc: ["copy@example.invalid"],
      bcc: ["private@example.invalid"],
      subject: "QA draft",
      text_body: "Draft body",
      attachments: [
        {
          filename: "details.txt",
          content: Buffer.from("QA attachment").toString("base64"),
        },
      ],
    })
    await command({ action: "send", id: draft.id })
    await command({ action: "send", id: draft.id })
    const queued = await service.retrieveMailMessage(String(draft.id))
    assert.equal(queued.folder, "outbox")

    // All provider requests in this process are intercepted. Existing operational mail must be absent.
    const otherPending = await service.listMailMessages(
      { folder: "outbox", account_id: { $ne: account.id } },
      { take: 1 }
    )
    assert.equal(
      otherPending.length,
      0,
      "Run queue QA only when no operational mail is pending"
    )
    const otherCampaigns = await service.listMailCampaigns(
      { account_id: { $ne: account.id }, status: ["queued", "sending"] },
      { take: 1 }
    )
    assert.equal(
      otherCampaigns.length,
      0,
      "Run queue QA only when no operational campaign is active"
    )
    const submissions: {
      path: string
      body: Record<string, any>
      idempotency: string | null
    }[] = []
    globalThis.fetch = (async (
      url: string | URL | Request,
      options?: RequestInit
    ) => {
      const path = String(url)
      if (!path.startsWith("https://api.resend.com"))
        return originalFetch(url, options)
      if (path.includes("/emails/receiving/"))
        return new Response(
          JSON.stringify({
            id: `${prefix}-incoming`,
            to: [account.email],
            cc: [],
            reply_to: ["buyer@example.invalid"],
            received_for: [],
            from: "Emma Laurent <buyer@example.invalid>",
            subject: "Question about iPhone 15 Pro",
            text: "Hello, could you confirm the battery health and shipping date for this order?",
            html: '<h2>Question about iPhone 15 Pro</h2><p>Hello, could you confirm the battery health and shipping date for this order?</p><img src="https://example.invalid/tracker.png">',
            message_id: `<${prefix}@example.invalid>`,
            attachments: [],
            created_at: new Date().toISOString(),
          }),
          { status: 200, headers: { "Content-Type": "application/json" } }
        )
      const body = JSON.parse(String(options?.body))
      submissions.push({
        path,
        body,
        idempotency: new Headers(options?.headers).get("idempotency-key"),
      })
      return new Response(
        JSON.stringify({ id: `${prefix}-provider-${submissions.length}` }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    }) as typeof fetch
    await processMailQueueWorkflow(container).run({ input: {} })
    assert.equal(submissions.length, 1)
    assert.equal(submissions[0].idempotency, `mail/${draft.id}`)
    assert.deepEqual(submissions[0].body.bcc, ["private@example.invalid"])
    assert.equal(
      (await service.retrieveMailMessage(String(draft.id))).folder,
      "sent"
    )
    const inbound = {
      type: "email.received" as const,
      created_at: new Date().toISOString(),
      data: {
        created_at: new Date().toISOString(),
        bcc: [],
        cc: [],
        received_for: [],
        email_id: `${prefix}-incoming`,
        from: "buyer@example.invalid",
        to: [account.email],
        subject: "Question",
        message_id: `<${prefix}@example.invalid>`,
        attachments: [],
      },
    }
    await mailWebhookWorkflow(container).run({
      input: {
        account_id: account.id,
        event_id: `${prefix}-event`,
        event: inbound,
      },
    })
    await mailWebhookWorkflow(container).run({
      input: {
        account_id: account.id,
        event_id: `${prefix}-event`,
        event: inbound,
      },
    })
    const incoming = await service.listMailMessages({
      account_id: account.id,
      direction: "inbound",
    })
    assert.equal(incoming.length, 1)
    assert.equal(incoming[0].read, false)
    assert.equal((await get("summary", `&account_id=${account.id}`)).unread, 1)
    await command({
      action: "messages",
      ids: [incoming[0].id],
      operation: "read",
    })
    assert.equal((await get("summary", `&account_id=${account.id}`)).unread, 0)
    await command({
      action: "messages",
      ids: [incoming[0].id],
      operation: "trash",
    })
    await command({
      action: "messages",
      ids: [incoming[0].id],
      operation: "restore",
    })
    assert.equal(
      (await service.retrieveMailMessage(incoming[0].id)).folder,
      "inbox"
    )
    assert.equal(
      (await get("messages", `&account_id=${account.id}&q=iPhone`)).count,
      1
    )
    const invalid = await originalFetch(`${base}/admin/mail/commands`, {
      method: "POST",
      headers,
      body: JSON.stringify({
        action: "save_draft",
        account_id: account.id,
        to: ["invalid"],
        subject: "Invalid",
      }),
    })
    assert.equal(invalid.status, 400)

    const contact = await service.createMailContacts({
      email: `${prefix}-sub@example.invalid`,
      name: "Subscribed contact",
      subscribed: true,
      consent_source: "QA opt-in",
      consent_at: new Date(),
      unsubscribe_token: randomBytes(32).toString("hex"),
    })
    const unsubscribed = await service.createMailContacts({
      email: `${prefix}-unsub@example.invalid`,
      name: "Unsubscribed contact",
      subscribed: false,
      unsubscribe_token: randomBytes(32).toString("hex"),
    })
    const suppressed = await service.createMailContacts({
      email: `${prefix}-suppressed@example.invalid`,
      name: "Suppressed contact",
      subscribed: true,
      suppressed: true,
      unsubscribe_token: randomBytes(32).toString("hex"),
    })
    const campaign = await service.createMailCampaigns({
      name: "Autumn arrivals",
      account_id: account.id,
      subject: "Discover our latest iPhones",
      text_body: "New arrivals are here.",
      created_by: user.id,
    })
    await mailCommandWorkflow(container).run({
      input: {
        action: "campaign",
        id: campaign.id,
        operation: "queue",
        contact_ids: [contact.id, unsubscribed.id, suppressed.id],
        actor_id: user.id,
      },
    })
    const deliveries = await service.listMailDeliveries({
      campaign_id: campaign.id,
    })
    assert.equal(
      deliveries.length,
      1,
      "Only eligible subscribers enter the queue"
    )
    await processMailQueueWorkflow(container).run({ input: {} })
    assert.equal(submissions.length, 2)
    assert.equal(submissions[1].body.to, contact.email)
    assert.ok(submissions[1].body.html.includes("Unsubscribe"))
    assert.equal(
      submissions[1].body.headers["List-Unsubscribe-Post"],
      "List-Unsubscribe=One-Click"
    )
    assert.equal(
      (await service.retrieveMailCampaign(campaign.id)).status,
      "completed"
    )
    // A scanner's GET must not unsubscribe anyone. POST performs the change.
    assert.equal(
      (
        await originalFetch(
          `${base}/mail/unsubscribe/${contact.unsubscribe_token}`
        )
      ).status,
      200
    )
    assert.equal(
      (await service.retrieveMailContact(contact.id)).subscribed,
      true
    )
    assert.equal(
      (
        await originalFetch(
          `${base}/mail/unsubscribe/${contact.unsubscribe_token}`,
          { method: "POST" }
        )
      ).status,
      200
    )
    assert.equal(
      (await service.retrieveMailContact(contact.id)).subscribed,
      false
    )
    await command({
      action: "contacts",
      contacts: [
        {
          email: contact.email,
          name: contact.name,
          subscribed: true,
          consent_source: "import",
        },
      ],
    })
    assert.equal(
      (await service.retrieveMailContact(contact.id)).subscribed,
      false,
      "Import cannot re-subscribe opted-out contacts"
    )
    const payload = JSON.stringify({
      type: "email.bounced",
      created_at: new Date().toISOString(),
      data: {
        email_id: `${prefix}-provider-2`,
        from: account.email,
        to: [contact.email],
        subject: "QA",
        bounce: { message: "QA bounce", type: "Permanent", subType: "General" },
      },
    })
    const eventId = `${prefix}-bounce`
    const timestamp = String(Math.floor(Date.now() / 1000))
    const signature = createHmac(
      "sha256",
      Buffer.from(secret.slice(6), "base64")
    )
      .update(`${eventId}.${timestamp}.${payload}`)
      .digest("base64")
    const signedHeaders = {
      "Content-Type": "application/json",
      "svix-id": eventId,
      "svix-timestamp": timestamp,
      "svix-signature": `v1,${signature}`,
    }
    assert.equal(
      (
        await originalFetch(`${base}/mail/webhooks/${account.id}`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: payload,
        })
      ).status,
      401
    )
    for (let i = 0; i < 2; i++)
      assert.equal(
        (
          await originalFetch(`${base}/mail/webhooks/${account.id}`, {
            method: "POST",
            headers: signedHeaders,
            body: payload,
          })
        ).status,
        204
      )
    assert.equal(
      (await service.retrieveMailContact(contact.id)).suppressed,
      true
    )
    assert.equal(
      (
        await service.listMailEvents({
          account_id: account.id,
          type: "email.bounced",
        })
      ).length,
      1
    )
    await service.updateMailMessages({ id: incoming[0].id, read: false })
    await service.createMailTemplates({
      name: `${prefix} / Order follow-up`,
      subject: "Your order update",
      text_body: "Hello, here is the latest update on your order.",
    })
    // Browser tests cover real authenticated rendering in both languages and mobile layout.
    if (process.env.MAIL_PLAYWRIGHT_MODULE && process.env.MAIL_QA_DIR) {
      const { chromium } = require(process.env.MAIL_PLAYWRIGHT_MODULE)
      browser = await chromium.launch({ headless: true })
      const context = await browser.newContext({
        viewport: { width: 1440, height: 1000 },
      })
      const session = await context.request.post(`${base}/auth/session`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      assert.equal(session.status(), 200)
      const page = await context.newPage()
      const pageErrors: string[] = []
      page.on("pageerror", (error: Error) => pageErrors.push(error.message))
      await mkdir(process.env.MAIL_QA_DIR, { recursive: true })
      for (const language of ["zhCN", "en"]) {
        await context.addCookies([{ name: "lng", value: language, url: base }])
        await page.goto(
          `${base}/app/mail?account=${account.id}&message=${incoming[0].id}`,
          { waitUntil: "networkidle" }
        )
        await page
          .getByRole("button", {
            name: language === "zhCN" ? "写邮件" : "Compose",
            exact: true,
          })
          .waitFor()
        await page
          .getByText("Question about iPhone 15 Pro", { exact: true })
          .first()
          .waitFor()
        await page.screenshot({
          path: join(process.env.MAIL_QA_DIR, `mail-${language}-desktop.png`),
          fullPage: true,
        })
        const screenshotName =
          language === "zhCN" ? "邮箱设置" : "Mailbox settings"
        await page
          .getByRole("link", { name: screenshotName, exact: true })
          .click()
        await page.waitForURL("**/app/settings/mailboxes")
        await page.getByText(account.email, { exact: true }).first().waitFor()
        await page.screenshot({
          path: join(process.env.MAIL_QA_DIR, `mail-settings-${language}.png`),
          fullPage: true,
        })
      }
      await page.goto(`${base}/app/mail?account=${account.id}`, {
        waitUntil: "networkidle",
      })
      await page.getByRole("button", { name: "Compose", exact: true }).click()
      await page
        .getByLabel("To", { exact: true })
        .fill("draft-browser@example.invalid")
      await page
        .getByLabel("Subject", { exact: true })
        .fill("Browser auto-save test")
      await page
        .getByLabel("Message", { exact: true })
        .fill("Browser autosave content")
      await page.getByText("Saved", { exact: true }).waitFor()
      await page
        .getByRole("button", { name: "Save draft", exact: true })
        .click()
      assert.equal(
        (
          await service.listMailMessages({
            account_id: account.id,
            subject: "Browser auto-save test",
          })
        ).length,
        1
      )
      await page.setViewportSize({ width: 390, height: 844 })
      await page.goto(`${base}/app/mail?account=${account.id}`, {
        waitUntil: "networkidle",
      })
      await page.screenshot({
        path: join(process.env.MAIL_QA_DIR, "mail-mobile.png"),
        fullPage: true,
      })
      assert.equal(
        await page.evaluate(
          () => document.documentElement.scrollWidth > window.innerWidth
        ),
        false,
        "No horizontal overflow on mobile"
      )
      assert.deepEqual(pageErrors, [], "No browser runtime errors")
      await browser.close()
      browser = undefined
    }
    console.log(
      "Mail QA passed: auth, credential masking, drafts, idempotent queueing, mocked delivery, inbound deduplication, read counts, trash/restore, search, validation, consent, unsubscribe, signed callbacks, suppression, and optional bilingual browser checks."
    )
  } finally {
    globalThis.fetch = originalFetch
    if (browser) await browser.close()
    const campaigns = await service.listMailCampaigns(
      { account_id: account.id },
      { take: 100 }
    )
    for (const campaign of campaigns) {
      const deliveries = await service.listMailDeliveries(
        { campaign_id: campaign.id },
        { take: 1000 }
      )
      if (deliveries.length)
        await service.deleteMailDeliveries(deliveries.map((row) => row.id))
    }
    if (campaigns.length)
      await service.deleteMailCampaigns(campaigns.map((row) => row.id))
    const messages = await service.listMailMessages(
      { account_id: account.id },
      { take: 1000 }
    )
    if (messages.length)
      await service.deleteMailMessages(messages.map((row) => row.id))
    const events = await service.listMailEvents(
      { account_id: account.id },
      { take: 1000 }
    )
    if (events.length)
      await service.deleteMailEvents(events.map((row) => row.id))
    const contacts = await service.listMailContacts(
      { email: { $like: `${prefix}%` } },
      { take: 1000 }
    )
    if (contacts.length)
      await service.deleteMailContacts(contacts.map((row) => row.id))
    const templates = await service.listMailTemplates(
      { name: { $like: `${prefix}%` } },
      { take: 1000 }
    )
    if (templates.length)
      await service.deleteMailTemplates(templates.map((row) => row.id))
    await service.deleteMailAccounts(account.id)
    if (originalPublicUrl === undefined) delete process.env.MAIL_PUBLIC_URL
    else process.env.MAIL_PUBLIC_URL = originalPublicUrl
  }
}
