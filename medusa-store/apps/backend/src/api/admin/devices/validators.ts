import { z } from "@medusajs/framework/zod"

const id = z.string().trim().min(1).max(100)
const optionalId = id.optional()
export const DeviceCategory = z.enum(["sale", "returned", "recycled"])
export const DevicePhoto = z.strictObject({ id, url: z.string().max(2000).refine((url) => /^https?:\/\//i.test(url) || /^\/static\/[a-zA-Z0-9._-]+$/.test(url)) })
const identity = {
  serial_number: z.string().trim().toUpperCase().min(1).max(100).optional(),
  imei: z
    .string()
    .trim()
    .regex(/^\d{15}$/)
    .optional(),
  imei2: z.string().trim().regex(/^\d{15}$/).optional(),
  eid: z.string().trim().regex(/^\d{32}$/).optional(),
}
export const IntakeDevice = z
  .object({
    ...identity,
    variant_id: id,
    source_id: id,
    category: DeviceCategory.default("sale"),
    photos: z.array(DevicePhoto).max(8).default([]),
    grade: z.enum(["S", "A", "B", "C"]).optional(),
    battery_health: z.number().int().min(0).max(100).optional(),
    cost_cents: z.number().int().min(0).max(100000000).default(0),
    currency_code: z
      .string()
      .regex(/^[a-z]{3}$/)
      .default("eur"),
    bin: z.string().trim().max(200).default(""),
    notes: z.string().trim().max(2000).default(""),
  })
  .strict()
  .refine((d) => d.serial_number, "请填写设备序列号")

export const UpdateDevice = z.strictObject({
  action: z.literal("update"),
  device_id: id,
  variant_id: id,
  source_id: id,
  location_id: id,
  imei: identity.imei.unwrap().nullable(),
  imei2: identity.imei2.unwrap().nullable(),
  eid: identity.eid.unwrap().nullable(),
  category: DeviceCategory,
  state: z.enum(["pending", "inspection", "available", "reserved", "packed", "shipped", "repair", "repair_ready", "retired", "supplier_returned"]),
  grade: z.enum(["S", "A", "B", "C"]).nullable(),
  battery_health: z.number().int().min(0).max(100).nullable(),
  cost_cents: z.number().int().min(0).max(100000000),
  currency_code: z.string().regex(/^[a-z]{3}$/),
  bin: z.string().trim().max(200),
  notes: z.string().trim().max(2000),
  photos: z.array(DevicePhoto).max(8),
})
export type UpdateDeviceBody = z.infer<typeof UpdateDevice>

export const DeviceCommand = z.discriminatedUnion("action", [
  z.strictObject({
    action: z.literal("save_allocation"),
    order_id: id,
    allocations: z.array(z.strictObject({ item_id: id, device_id: id, assignment_id: optionalId, fulfillment_id: optionalId })).min(1).max(100),
  }),
  UpdateDevice,
  z.strictObject({ action: z.literal("activate"), device_id: id }),
  z.strictObject({ action: z.literal("make_available"), device_ids: z.array(id).min(1).max(200).refine((ids) => new Set(ids).size === ids.length, "Duplicate device IDs") }),
  z.strictObject({ action: z.literal("delete"), device_id: id }),
  z.strictObject({ action: z.literal("retire"), device_id: id }),
  z.strictObject({
    action: z.literal("order_assign"),
    order_id: id,
    item_id: id,
    serial_number: z.string().trim().toUpperCase().min(1).max(100),
    imei: identity.imei,
    imei2: identity.imei2,
    eid: identity.eid,
    device_id: optionalId,
    source_id: optionalId,
  }),
  z
    .object({
      action: z.literal("source"),
      name: z.string().trim().min(1).max(100),
      type: z.enum(["self", "supplier"]),
    })
    .strict(),
  z
    .object({
      action: z.literal("enroll"),
      variant_id: id,
      acknowledged: z.literal(true),
    })
    .strict(),
  z
    .object({
      action: z.literal("intake"),
      devices: z.array(IntakeDevice).min(1).max(200),
    })
    .strict(),
  z
    .object({
      action: z.literal("supply"),
      source_id: id,
      variant_id: id,
      quantity: z.number().int().min(0).max(100000),
    })
    .strict(),
  z
    .object({
      action: z.literal("assign"),
      device_id: id,
      order_id: id,
      item_id: id,
      replacement_of: optionalId,
    })
    .strict(),
  z
    .object({
      action: z.literal("supplier_assign"),
      device: IntakeDevice,
      order_id: id,
      item_id: id,
      replacement_of: optionalId,
    })
    .strict(),
  z.object({ action: z.literal("release"), assignment_id: id }).strict(),
  z
    .object({
      action: z.literal("fulfill"),
      assignment_ids: z.array(id).min(1).max(100),
    })
    .strict(),
  z
    .object({
      action: z.literal("ship"),
      fulfillment_id: id,
      tracking_number: z.string().trim().min(1).max(200),
      tracking_url: z
        .string()
        .url()
        .refine((url) => /^https?:\/\//i.test(url))
        .optional(),
    })
    .strict(),
  z
    .object({ action: z.literal("cancel_fulfillment"), fulfillment_id: id })
    .strict(),
  z
    .object({
      action: z.literal("receive"),
      assignment_id: id,
      scan: id,
      source_id: id,
      request_id: id,
    })
    .strict(),
  z
    .object({
      action: z.literal("inspect"),
      device_id: id,
      result: z.enum(["pass", "repair", "retire", "supplier_return"]),
      grade: z.enum(["S", "A", "B", "C"]),
      battery_health: z.number().int().min(0).max(100),
      variant_id: optionalId,
      notes: z.string().trim().min(1).max(2000),
    })
    .strict(),
  z
    .object({
      action: z.literal("repair_ship"),
      assignment_id: id,
      tracking_number: z.string().trim().min(1).max(200),
    })
    .strict(),
])

export type DeviceCommandBody = z.infer<typeof DeviceCommand>
export type IntakeDeviceBody = z.infer<typeof IntakeDevice>

export const DeviceRefund = z
  .object({
    request_id: id,
    payment_id: id,
    amount: z.number().positive().max(1000000),
    confirmed: z.literal(true),
  })
  .strict()
export type DeviceRefundBody = z.infer<typeof DeviceRefund>
