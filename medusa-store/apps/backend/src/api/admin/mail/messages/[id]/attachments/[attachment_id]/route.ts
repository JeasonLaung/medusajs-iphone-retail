import type {
  AuthenticatedMedusaRequest,
  MedusaResponse,
} from "@medusajs/framework/http"
import { MedusaError } from "@medusajs/framework/utils"
import { Resend } from "resend"
import MailModuleService from "../../../../../../../modules/mail/service"
import { MAIL_MODULE } from "../../../../../../../modules/mail"
import { decryptSecret } from "../../../../../../../modules/mail/security"

export async function GET(
  req: AuthenticatedMedusaRequest,
  res: MedusaResponse
) {
  const service: MailModuleService = req.scope.resolve(MAIL_MODULE)
  const message = await service.retrieveMailMessage(req.params.id)
  const attachments = message.attachments as unknown as {
    id?: string
    filename: string
    content?: string
  }[]
  const attachment = attachments.find(
    (a, i) => (a.id || String(i)) === req.params.attachment_id
  )
  if (!attachment)
    throw new MedusaError(
      MedusaError.Types.NOT_FOUND,
      "mail.attachmentNotFound"
    )
  if (attachment.content) {
    res.setHeader("Content-Type", "application/octet-stream")
    res.setHeader("X-Content-Type-Options", "nosniff")
    res.setHeader(
      "Content-Disposition",
      `attachment; filename*=UTF-8''${encodeURIComponent(attachment.filename)}`
    )
    return res.send(Buffer.from(attachment.content, "base64"))
  }
  const account = await service.retrieveMailAccount(message.account_id)
  const resend = new Resend(decryptSecret(account.api_key))
  const { data, error } = await resend.emails.receiving.attachments.get({
    emailId: message.provider_id!,
    id: attachment.id!,
  })
  if (error || !data)
    throw new MedusaError(
      MedusaError.Types.UNEXPECTED_STATE,
      "mail.attachmentUnavailable"
    )
  res.json({ url: data.download_url })
}
