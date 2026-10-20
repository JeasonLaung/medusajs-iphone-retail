import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import type { DevicePhotoUploadBody } from "../../../device-photo-validators"
import { uploadDevicePhotosWorkflow } from "../../../../workflows/device-photos"
import { devicePhotoUrl } from "../../../../lib/device-photo-url"

export async function POST(req: AuthenticatedMedusaRequest<DevicePhotoUploadBody>, res: MedusaResponse) {
  const { result } = await uploadDevicePhotosWorkflow(req.scope).run({ input: { ...req.validatedBody, actor_id: req.auth_context.actor_id } })
  res.status(201).json({ photos: result.map(({ id, url }) => ({ id, url: devicePhotoUrl(url) })) })
}
