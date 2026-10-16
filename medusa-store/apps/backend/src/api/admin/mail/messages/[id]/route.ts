import type {
  AuthenticatedMedusaRequest,
  MedusaResponse,
} from "@medusajs/framework/http"
import MailModuleService from "../../../../../modules/mail/service"
import { MAIL_MODULE } from "../../../../../modules/mail"

export async function GET(
  req: AuthenticatedMedusaRequest,
  res: MedusaResponse
) {
  const service: MailModuleService = req.scope.resolve(MAIL_MODULE)
  res.json({ message: await service.retrieveMailMessage(req.params.id) })
}
