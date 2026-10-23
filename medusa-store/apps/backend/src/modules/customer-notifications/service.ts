import { MedusaService } from "@medusajs/framework/utils"
import { CustomerNotice } from "./models/customer-notice"

export default class CustomerNotificationsService extends MedusaService({ CustomerNotice }) {}
