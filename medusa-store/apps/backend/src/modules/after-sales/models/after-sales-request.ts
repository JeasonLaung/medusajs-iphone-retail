import { model } from "@medusajs/framework/utils"

const AfterSalesRequest = model.define("after_sales_request", {
  id: model.id({ prefix: "asr" }).primaryKey(),
  order_id: model.text(),
  customer_id: model.text(),
  type: model.enum(["return", "exchange", "repair"]),
  reason: model.text(),
  items: model.json(),
  status: model.enum(["pending", "processing", "completed", "rejected"]).default("pending"),
}).indexes([
  { on: ["order_id"] },
  { name: "IDX_after_sales_one_open_request", on: ["order_id"], unique: true, where: "status IN ('pending', 'processing') AND deleted_at IS NULL" },
])

export default AfterSalesRequest
