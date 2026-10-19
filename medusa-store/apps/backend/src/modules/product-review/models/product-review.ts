import { model } from "@medusajs/framework/utils"

const ProductReview = model.define("product_review", {
  id: model.id({ prefix: "rev" }).primaryKey(),
  product_id: model.text(),
  customer_id: model.text(),
  order_id: model.text().nullable(),
  verified_purchase: model.boolean().default(false),
  author: model.text(),
  rating: model.number(),
  title: model.text(),
  content: model.text(),
  status: model.enum(["pending", "approved", "rejected"]).default("pending"),
}).indexes([
  { on: ["product_id", "customer_id"], unique: true, where: "deleted_at IS NULL" },
  { on: ["product_id", "status"] },
])

export default ProductReview
