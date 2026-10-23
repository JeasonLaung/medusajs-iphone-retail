import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { Modules } from "@medusajs/framework/utils"
import type { IFileModuleService } from "@medusajs/framework/types"
import { BUYBACK_MODULE } from "../modules/buyback"
import type BuybackModuleService from "../modules/buyback/service"
import { retentionDays } from "../lib/buyback"

// Only closed applications expire. Delete the private object first so failures
// remain retryable; an already deleted local file can safely be deleted again.
const expireBuybackPhotos = createStep("expire-buyback-photos", async (_, { container }) => {
  const service = container.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const files = container.resolve<IFileModuleService>(Modules.FILE)
  const cutoff = new Date(Date.now() - retentionDays * 86400000)
  let skip = 0
  let expired = 0
  while (true) {
    const requests = await service.listBuybackRequests({ closed_at: { $lt: cutoff } }, { take: 100, skip, order: { id: "ASC" } })
    if (!requests.length) break
    for (const request of requests) {
      const photos = await service.listBuybackPhotos({ request_id: request.id }, { take: null })
      for (const photo of photos) {
        if (photo.review_status === "expired") continue
        if (photo.storage_key) await files.deleteFiles(photo.storage_key)
        await service.updateBuybackPhotos({ id: photo.id, storage_key: "", review_status: "expired" })
        expired++
      }
    }
    skip += requests.length
  }
  return new StepResponse({ expired })
})
export const expireBuybackPhotosWorkflow = createWorkflow("expire-buyback-photos", function () {
  return new WorkflowResponse(expireBuybackPhotos())
})
