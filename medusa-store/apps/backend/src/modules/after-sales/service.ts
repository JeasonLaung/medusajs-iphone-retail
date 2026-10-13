import { MedusaService } from "@medusajs/framework/utils"
import AfterSalesRequest from "./models/after-sales-request"

class AfterSalesModuleService extends MedusaService({ AfterSalesRequest }) {}

export default AfterSalesModuleService
