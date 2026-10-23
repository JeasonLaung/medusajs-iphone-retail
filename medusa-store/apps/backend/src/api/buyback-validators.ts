import { z } from "@medusajs/framework/zod"
import { completeAssessment } from "../lib/recycled-assessment"

const id = z.string().trim().min(1).max(100)
const money = z.number().finite().min(0).max(1000000).multipleOf(0.01)
export const BuybackAssessment = z.strictObject({
  battery: z.string().max(7), checks: z.array(z.boolean()).length(5),
  faults: z.array(z.enum(["bright", "pixels", "lines", "none"])).min(1).max(3),
  glass: z.array(z.enum(["cracked", "chipped", "scratched", "none"])).min(1).max(3),
  screen: z.string().max(10), frame: z.string().max(10), back: z.string().max(10),
}).refine(completeAssessment, "BUYBACK_INVALID_ASSESSMENT")
export const ReturnAddress = z.strictObject({
  name: z.string().trim().min(2).max(100), address_1: z.string().trim().min(3).max(200),
  address_2: z.string().trim().max(200).default(""), city: z.string().trim().min(1).max(100),
  postal_code: z.string().trim().min(1).max(30), country_code: z.string().regex(/^[a-z]{2}$/),
})
export const CreateBuyback = z.strictObject({
  submission_key: z.uuid(), contact_name: z.string().trim().min(2).max(100),
  phone: z.string().trim().min(5).max(50), return_address: ReturnAddress,
  reference_acknowledged: z.literal(true), privacy_acknowledged: z.literal(true),
  items: z.array(z.strictObject({ model: z.string().trim().min(1).max(150), capacity: z.string().regex(/^\d+(GB|TB)$/), assessment: BuybackAssessment })).min(1).max(10),
})
export type CreateBuyback = z.infer<typeof CreateBuyback>
const identity = z.strictObject({ item_id: id, serial_number: z.string().trim().toUpperCase().min(1).max(100), imei: z.string().regex(/^\d{15}$/) })
export const BuybackCustomerCommand = z.discriminatedUnion("action", [
  z.strictObject({ action: z.literal("submit_photos"), items: z.array(identity).min(1).max(10) }),
  z.strictObject({ action: z.literal("mark_shipped"), tracking_number: z.string().trim().min(3).max(150) }),
  z.strictObject({ action: z.literal("accept_offer"), final_amount: money }),
  z.strictObject({ action: z.literal("decline_offer") }),
  z.strictObject({ action: z.literal("cancel") }),
])
export type BuybackCustomerCommand = z.infer<typeof BuybackCustomerCommand>
export const BuybackAdminCommand = z.discriminatedUnion("action", [
  z.strictObject({ action: z.literal("review_photo"), photo_id: id, review_status: z.enum(["approved", "retake"]), note: z.string().trim().max(500).default("") }),
  z.strictObject({ action: z.literal("approve_shipping"), shipping_instructions: z.string().trim().min(5).max(2000) }),
  z.strictObject({ action: z.literal("mark_received") }),
  z.strictObject({ action: z.literal("link_device"), item_id: id, device_id: id }),
  z.strictObject({ action: z.literal("final_offer"), items: z.array(z.strictObject({ item_id: id, amount: money, notes: z.string().trim().min(3).max(1000) })).min(1).max(10) }),
  z.strictObject({ action: z.literal("mark_paid"), payment_reference: z.string().trim().min(3).max(200) }),
  z.strictObject({ action: z.literal("request_return"), note: z.string().trim().min(3).max(1000) }),
  z.strictObject({ action: z.literal("mark_returning"), return_tracking: z.string().trim().min(3).max(150) }),
  z.strictObject({ action: z.literal("mark_returned") }),
  z.strictObject({ action: z.literal("cancel"), note: z.string().trim().min(3).max(1000) }),
])
export type BuybackAdminCommand = z.infer<typeof BuybackAdminCommand>
export const BuybackReferencePrice = z.strictObject({ product_id: id, capacity: z.string().regex(/^\d+(GB|TB)$/), amount: money })
export type BuybackReferencePrice = z.infer<typeof BuybackReferencePrice>
export const BuybackPhotoUpload = z.strictObject({ item_id: id, photo_type: z.enum(["front", "back", "edges", "damage", "packaging", "sealed"]), mime_type: z.enum(["image/jpeg", "image/png", "image/webp"]), content: z.string().min(4).max(4200000).regex(/^[A-Za-z0-9+/]+={0,2}$/) })
export type BuybackPhotoUpload = z.infer<typeof BuybackPhotoUpload>
