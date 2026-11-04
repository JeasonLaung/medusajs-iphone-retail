import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import MailModuleService from "../modules/mail/service"
import { MAIL_MODULE } from "../modules/mail"
import { decryptSecret } from "../modules/mail/security"
import { RelaymeEvent, verifyRelayme } from "../modules/mail/relayme"
import { relaymeWebhookWorkflow } from "../workflows/relayme-webhook"

export async function receiveRelayme(req: MedusaRequest, res: MedusaResponse, kind: "inbound" | "delivery") {
  const keyId = req.get("X-Relayme-Key-ID") || ""
  if (!keyId || keyId.length > 255) return res.sendStatus(401)
  const service: MailModuleService = req.scope.resolve(MAIL_MODULE)
  const accounts = await service.listMailAccounts({ provider: "relayme", relayme_key_id: keyId, enabled: true }, { take: 1000 })
  if (!accounts.length) return res.sendStatus(401)
  // bodyParser is disabled on these endpoints. Never parse JSON before verifying the raw bytes.
  const chunks: Buffer[] = []
  let size = 0
  for await (const chunk of req) {
    size += chunk.length
    if (size > 1024 * 1024) return res.sendStatus(413)
    chunks.push(Buffer.from(chunk))
  }
  const rawBody = Buffer.concat(chunks)
  const verified = accounts.filter((a) => a.webhook_secret && verifyRelayme({
    method: req.method, path: req.originalUrl.split("?")[0], rawBody,
    timestamp: req.get("X-Relayme-Timestamp") || "",
    nonce: req.get("X-Relayme-Nonce") || "",
    signature: req.get("X-Relayme-Signature") || "",
    secret: decryptSecret(a.webhook_secret),
  }))
  if (!verified.length) return res.sendStatus(401)
  let parsed: unknown
  try { parsed = JSON.parse(rawBody.toString("utf8")) } catch { return res.sendStatus(400) }
  const result = RelaymeEvent.safeParse(parsed)
  if (!result.success || req.get("X-Relayme-Event-ID") !== result.data.event_id) return res.sendStatus(400)
  if ((kind === "inbound") !== ("username" in result.data)) return res.sendStatus(400)
  await relaymeWebhookWorkflow(req.scope).run({ input: { account_ids: verified.map((a) => a.id), key_id: keyId, event: result.data } })
  return res.sendStatus(204)
}
