import { model } from "@medusajs/framework/utils"

export const BuybackRequest = model.define("buyback_request", {
  id: model.id({ prefix: "bbr" }).primaryKey(),
  customer_id: model.text(),
  submission_key: model.text(),
  contact_name: model.text(),
  email: model.text(),
  phone: model.text(),
  return_address: model.json(),
  items: model.json(),
  status: model.enum(["awaiting_photos", "photo_review", "ready_to_ship", "in_transit", "received", "final_offer", "accepted", "paid", "return_requested", "returning", "returned", "canceled"]).default("awaiting_photos"),
  reference_amount: model.bigNumber().nullable(),
  final_amount: model.bigNumber().nullable(),
  currency_code: model.text().default("eur"),
  shipping_instructions: model.text().default(""),
  tracking_number: model.text().default(""),
  return_tracking: model.text().default(""),
  payment_reference: model.text().default(""),
  review_note: model.text().default(""),
  history: model.json().default([] as unknown as Record<string, unknown>),
  closed_at: model.dateTime().nullable(),
}).indexes([
  { on: ["customer_id", "created_at"] },
  { on: ["status", "created_at"] },
  { on: ["customer_id", "submission_key"], unique: true },
])

export const BuybackPhoto = model.define("buyback_photo", {
  id: model.id({ prefix: "bbp" }).primaryKey(),
  request_id: model.text(),
  item_id: model.text(),
  photo_type: model.enum(["front", "back", "edges", "damage", "packaging", "sealed"]),
  uploaded_at: model.dateTime(),
  storage_key: model.text(),
  mime_type: model.text(),
  review_status: model.enum(["pending", "approved", "retake", "expired"]).default("pending"),
  review_note: model.text().default(""),
}).indexes([{ on: ["request_id", "item_id"] }])

export const BuybackPrice = model.define("buyback_price", {
  id: model.id({ prefix: "bbv" }).primaryKey(),
  product_id: model.text(),
  capacity: model.text(),
  amount: model.bigNumber(),
  pricing_rules: model.json().nullable(),
  currency_code: model.text().default("eur"),
}).indexes([{ on: ["product_id", "capacity"], unique: true }])

export const BuybackDeviceLink = model.define("buyback_device_link", {
  id: model.id({ prefix: "bbl" }).primaryKey(),
  request_id: model.text(), item_id: model.text(), device_id: model.text(),
}).indexes([{ on: ["item_id"], unique: true }, { on: ["device_id"], unique: true }])
