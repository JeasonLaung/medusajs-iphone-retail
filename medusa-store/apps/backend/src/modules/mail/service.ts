import { MedusaService } from "@medusajs/framework/utils"
import {
  MailAccount,
  MailMessage,
  MailContact,
  MailCampaign,
  MailDelivery,
  MailEvent,
  MailTemplate,
} from "./models/mail"

export default class MailModuleService extends MedusaService({
  MailAccount,
  MailMessage,
  MailContact,
  MailCampaign,
  MailDelivery,
  MailEvent,
  MailTemplate,
}) {}
