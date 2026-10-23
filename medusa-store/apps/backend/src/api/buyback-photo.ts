import type { MedusaContainer, IFileModuleService } from "@medusajs/framework/types"
import { MedusaError, Modules } from "@medusajs/framework/utils"
import { BUYBACK_MODULE } from "../modules/buyback"
import type BuybackModuleService from "../modules/buyback/service"

// Called only after authorizing the enclosing application. Storage keys and
// provider URLs are never returned to either the customer or dashboard.
export async function readBuybackPhoto(container: MedusaContainer, requestId: string, photoId: string) {
  const service = container.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const [photo] = await service.listBuybackPhotos({ id: photoId, request_id: requestId })
  if (!photo || photo.review_status === "expired" || !photo.storage_key) throw new MedusaError(MedusaError.Types.NOT_FOUND, "BUYBACK_PHOTO_NOT_FOUND")
  const content = await container.resolve<IFileModuleService>(Modules.FILE).getAsBuffer(photo.storage_key)
  return { content: content.toString("base64"), mime_type: photo.mime_type }
}
