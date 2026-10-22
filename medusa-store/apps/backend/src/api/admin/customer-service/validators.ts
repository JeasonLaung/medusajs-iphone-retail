import { z } from "@medusajs/framework/zod"

const time = z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/)
export const ServiceContact = z.object({
  id: z.string().min(1).max(80),
  name: z.string().trim().min(1).max(100),
  department: z.enum(["pre_sales", "after_sales"]),
  whatsapp: z.string().trim().max(30).refine(value => !value || /^\+?[1-9]\d{6,14}$/.test(value), "SERVICE_WHATSAPP_INVALID"),
  email: z.union([z.email(), z.literal("")]),
  timezone: z.string().max(80).refine(value => {
    try { new Intl.DateTimeFormat("en", { timeZone: value }); return Boolean(value) } catch { return false }
  }, "SERVICE_TIMEZONE_INVALID"),
  days: z.array(z.number().int().min(0).max(6)).min(1).max(7),
  start: time,
  end: time,
  availability: z.enum(["schedule", "online", "offline"]),
  enabled: z.boolean(),
}).refine(value => Boolean(value.whatsapp || value.email), "SERVICE_CHANNEL_REQUIRED")
  .refine(value => value.start < value.end, "SERVICE_HOURS_INVALID")

export const CustomerServiceBody = z.object({
  contacts: z.array(ServiceContact).max(100),
}).refine(value => new Set(value.contacts.map(contact => contact.id)).size === value.contacts.length, "SERVICE_DUPLICATE_ID")
export type CustomerServiceBody = z.infer<typeof CustomerServiceBody>
export type ServiceContact = z.infer<typeof ServiceContact>
