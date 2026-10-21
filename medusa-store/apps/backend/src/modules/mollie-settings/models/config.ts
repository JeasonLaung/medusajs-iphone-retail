import { model } from "@medusajs/framework/utils"

export const MollieConfig = model.define("mollie_config", {
  id: model.id({ prefix: "molliecfg" }).primaryKey(),
  environment: model.enum(["test", "production"]).default("test"),
  api_key: model.text().default(""),
  redirect_url: model.text().default(""),
  enabled: model.boolean().default(false),
})
