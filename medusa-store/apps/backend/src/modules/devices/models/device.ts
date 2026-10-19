import { model } from "@medusajs/framework/utils"

export const Device = model
  .define("device", {
    id: model.id({ prefix: "dev" }).primaryKey(),
    serial_number: model.text().nullable(),
    imei: model.text().nullable(),
    imei2: model.text().nullable(),
    variant_id: model.text(),
    source_id: model.text(),
    location_id: model.text(),
    grade: model.text().nullable(),
    battery_health: model.number().nullable(),
    cost_cents: model.number().default(0),
    currency_code: model.text().default("eur"),
    bin: model.text().default(""),
    notes: model.text().default(""),
    state: model
      .enum([
        "inspection",
        "available",
        "reserved",
        "packed",
        "shipped",
        "repair",
        "repair_ready",
        "retired",
        "supplier_returned",
      ])
      .default("inspection"),
  })
  .indexes([
    { on: ["serial_number"], unique: true, where: "serial_number IS NOT NULL" },
    { on: ["imei"], unique: true, where: "imei IS NOT NULL" },
    { on: ["imei2"], unique: true, where: "imei2 IS NOT NULL" },
    { on: ["variant_id", "location_id", "state"] },
  ])

export const DeviceSource = model
  .define("device_source", {
    id: model.id({ prefix: "dsrc" }).primaryKey(),
    name: model.text(),
    type: model.enum(["self", "supplier"]),
    location_id: model.text(),
  })
  .indexes([{ on: ["location_id"], unique: true }])

export const DeviceVariant = model
  .define("device_variant", {
    id: model.id({ prefix: "dvar" }).primaryKey(),
    variant_id: model.text(),
    inventory_item_id: model.text(),
  })
  .indexes([
    { on: ["variant_id"], unique: true },
    { on: ["inventory_item_id"], unique: true },
  ])

export const DeviceSupply = model
  .define("device_supply", {
    id: model.id({ prefix: "dsup" }).primaryKey(),
    source_id: model.text(),
    variant_id: model.text(),
    quantity: model.number().default(0),
  })
  .indexes([{ on: ["source_id", "variant_id"], unique: true }])

export const DeviceAssignment = model
  .define("device_assignment", {
    id: model.id({ prefix: "dasg" }).primaryKey(),
    device_id: model.text(),
    order_id: model.text(),
    item_id: model.text(),
    source_id: model.text(),
    status: model.enum([
      "reserved",
      "packed",
      "shipped",
      "returned",
      "canceled",
    ]),
    fulfillment_id: model.text().nullable(),
    return_id: model.text().nullable(),
    after_sales_request_id: model.text().nullable(),
    replacement_of: model.text().nullable(),
    tracking_number: model.text().nullable(),
  })
  .indexes([
    {
      on: ["device_id"],
      unique: true,
      where:
        "status IN ('reserved', 'packed', 'shipped') AND deleted_at IS NULL",
    },
    { on: ["order_id", "item_id"] },
  ])

export const DeviceEvent = model
  .define("device_event", {
    id: model.id({ prefix: "devt" }).primaryKey(),
    device_id: model.text().nullable(),
    order_id: model.text().nullable(),
    actor_id: model.text(),
    action: model.text(),
    data: model.json(),
  })
  .indexes([{ on: ["device_id"] }, { on: ["order_id"] }])
