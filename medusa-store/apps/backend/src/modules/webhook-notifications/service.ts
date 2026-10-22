import { MedusaService } from "@medusajs/framework/utils"
import { WebhookEndpoint } from "./models/endpoint"
export default class WebhookNotificationsService extends MedusaService({ WebhookEndpoint }) {}
