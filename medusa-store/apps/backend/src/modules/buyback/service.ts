import { MedusaService } from "@medusajs/framework/utils"
import type { ModelConfigurationsToConfigTemplate } from "@medusajs/framework/utils"
import { BuybackRequest, BuybackPhoto, BuybackPrice, BuybackDeviceLink } from "./models/buyback-request"

const models = { BuybackRequest, BuybackPhoto, BuybackPrice, BuybackDeviceLink }
type ModelConfig = ModelConfigurationsToConfigTemplate<typeof models>
// Medusa's type-level pluralizer infers "Photoes", while runtime uses "Photos".
type BuybackConfig = Omit<ModelConfig, "BuybackPhoto"> & {
  BuybackPhoto: ModelConfig["BuybackPhoto"] & { plural: "BuybackPhotos" }
}

export default class BuybackModuleService extends MedusaService<BuybackConfig>(models) {}
