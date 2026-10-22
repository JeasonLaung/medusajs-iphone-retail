import { model } from "@medusajs/framework/utils"

export const WebhookEndpoint = model.define("webhook_endpoint", {
  id: model.id({ prefix: "whk" }).primaryKey(),
  name: model.text(),
  provider: model.enum(["feishu", "wecom", "dingtalk", "slack", "mattermost", "generic"]),
  url: model.text(),
  secret: model.text().default(""),
  token: model.text().default(""),
  keyword: model.text().default(""),
  enabled: model.boolean().default(true),
  events: model.json(),
  language: model.enum(["zh", "en"]).default("zh"),
  last_status: model.text().default(""),
  last_sent_at: model.dateTime().nullable(),
})
