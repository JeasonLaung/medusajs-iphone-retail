import { MedusaError } from "@medusajs/framework/utils"
import type { MedusaContainer, InferTypeOf } from "@medusajs/framework/types"
import { BuybackRequest, BuybackPhoto } from "../modules/buyback/models/buyback-request"
import { BUYBACK_MODULE } from "../modules/buyback"
import type BuybackModuleService from "../modules/buyback/service"
import type { RecycledAssessment } from "./recycled-assessment"
import { deviceAccess } from "../api/device-access"
import { warehouseAccess } from "./warehouse-access"

export type BuybackItem = {
  id: string; product_id: string; model: string; model_name: string; capacity: string
  assessment: RecycledAssessment; reference_amount: number | null; final_amount: number | null
  inspection_notes: string; serial_number: string; imei: string; device_id: string | null
}
export type RequestRecord = InferTypeOf<typeof BuybackRequest>
export type PhotoRecord = InferTypeOf<typeof BuybackPhoto>
export const requestItems = (request: { items: unknown }) => request.items as BuybackItem[]
export const retentionDays = 180
export const requiredPhotos = (item: BuybackItem) => ["front", "back", ...(["damaged", "visible"].includes(item.assessment.frame) ? ["edges"] : [])]
export function buybackError(code: string): never { throw new MedusaError(MedusaError.Types.NOT_ALLOWED, code) }
export function assertBuyback(condition: unknown, code = "BUYBACK_INVALID_STATE"): asserts condition { if (!condition) buybackError(code) }
export function latestPhotos(photos: PhotoRecord[]) {
  const latest = new Map<string, PhotoRecord>()
  for (const photo of [...photos].sort((a, b) => new Date(a.uploaded_at).getTime() - new Date(b.uploaded_at).getTime() || a.id.localeCompare(b.id))) latest.set(`${photo.item_id}:${photo.photo_type}`, photo)
  return [...latest.values()]
}
export function assertPhotos(items: BuybackItem[], photos: PhotoRecord[], approved = false) {
  const current = latestPhotos(photos)
  assertBuyback(items.every((item) => requiredPhotos(item).every((type) => current.some((photo) => photo.item_id === item.id && photo.photo_type === type && photo.review_status !== "expired" && (approved ? photo.review_status === "approved" : photo.review_status !== "retake")))), "BUYBACK_PHOTOS_REQUIRED")
}
export async function adminBuybackAccess(container: MedusaContainer, actorId: string) {
  const access = await deviceAccess(container, actorId)
  if (access.role !== "admin" || (await warehouseAccess(container, actorId)).warehouse) throw new MedusaError(MedusaError.Types.FORBIDDEN, "BUYBACK_DENIED")
}
export async function customerBuyback(container: MedusaContainer, id: string, customerId: string) {
  const service = container.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const [request] = await service.listBuybackRequests({ id, customer_id: customerId })
  if (!request) throw new MedusaError(MedusaError.Types.NOT_FOUND, "BUYBACK_NOT_FOUND")
  return request
}
export function publicPhoto(photo: PhotoRecord) {
  return { id: photo.id, request_id: photo.request_id, item_id: photo.item_id, photo_type: photo.photo_type, uploaded_at: photo.uploaded_at, review_status: photo.review_status, review_note: photo.review_note, mime_type: photo.mime_type }
}
export async function serializeBuyback(container: MedusaContainer, request: RequestRecord) {
  const service = container.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const photos = await service.listBuybackPhotos({ request_id: request.id }, { take: null })
  const { submission_key, customer_id, ...safe } = request
  return { ...safe, reference_amount: request.reference_amount == null ? null : Number(request.reference_amount), final_amount: request.final_amount == null ? null : Number(request.final_amount), items: requestItems(request), photos: latestPhotos(photos).map(publicPhoto), retention_days: retentionDays }
}
