import { model } from "@medusajs/framework/utils"

export const CustomerNotice = model.define("customer_notice", {
  id: model.id({ prefix: "notice" }).primaryKey(),
  customer_id: model.text(),
  event_key: model.text(),
  kind: model.text(),
  resource_id: model.text(),
  reference: model.text(),
  action: model.text().default(""),
  read_at: model.dateTime().nullable(),
}).indexes([
  { on: ["event_key"], unique: true },
  { on: ["customer_id", "created_at"] },
  { on: ["customer_id", "read_at"] },
])
