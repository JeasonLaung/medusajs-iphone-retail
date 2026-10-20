import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import type { DevicePhotoUploadBody } from "../../device-photo-validators"
import { uploadDevicePhotosWorkflow } from "../../../workflows/device-photos"
import { devicePhotoUrl } from "../../../lib/device-photo-url"

export async function POST(req: MedusaRequest<DevicePhotoUploadBody>, res: MedusaResponse) {
  const { result } = await uploadDevicePhotosWorkflow(req.scope).run({ input: { ...req.validatedBody, token: String(req.headers["x-intake-token"] || "") } })
  res.setHeader("Cache-Control", "no-store")
  res.status(201).json({ photos: result.map(({ id, url }) => ({ id, url: devicePhotoUrl(url) })) })
}
