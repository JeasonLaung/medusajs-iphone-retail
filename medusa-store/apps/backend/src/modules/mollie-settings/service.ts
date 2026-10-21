import { MedusaService } from "@medusajs/framework/utils"
import { MollieConfig } from "./models/config"

export default class MollieSettingsModuleService extends MedusaService({
  MollieConfig,
}) {}
