import { randomUUID } from "node:crypto"
import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { MedusaError } from "@medusajs/framework/utils"
import { uploadFilesWorkflow } from "@medusajs/medusa/core-flows"
import { deviceAccess } from "../api/device-access"
import { mobileIntakeAccess } from "../api/mobile-intake/access"
import type { DevicePhotoUploadBody } from "../api/device-photo-validators"

type Input = DevicePhotoUploadBody & ({ actor_id: string; token?: never } | { token: string; actor_id?: never })
const preparePhotos = createStep("prepare-photos", async (input: Input, { container }) => {
  if ("token" in input) await mobileIntakeAccess(container, input.token || "")
  else if (!input.actor_id || (await deviceAccess(container, input.actor_id)).role !== "admin")
    throw new MedusaError(MedusaError.Types.FORBIDDEN, "DEVICE_PHOTO_DENIED")
  const files = input.files.map((file) => {
    const bytes = Buffer.from(file.content, "base64")
    const mime = bytes.subarray(0, 3).equals(Buffer.from([255, 216, 255])) ? "image/jpeg"
      : bytes.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])) ? "image/png"
      : bytes.toString("ascii", 0, 4) === "RIFF" && bytes.toString("ascii", 8, 12) === "WEBP" ? "image/webp" : ""
    if (!mime || mime !== file.mime_type || bytes.length > 3 * 1024 * 1024)
      throw new MedusaError(MedusaError.Types.INVALID_DATA, "DEVICE_PHOTO_INVALID")
    return { filename: `device-${randomUUID()}.${mime === "image/jpeg" ? "jpg" : mime.split("/")[1]}`, mimeType: mime, content: file.content, access: "public" as const }
  })
  return new StepResponse({ files })
})
export const uploadDevicePhotosWorkflow = createWorkflow("upload-device-photos", (input: Input) => {
  const files = preparePhotos(input)
  return new WorkflowResponse(uploadFilesWorkflow.runAsStep({ input: files }))
})
