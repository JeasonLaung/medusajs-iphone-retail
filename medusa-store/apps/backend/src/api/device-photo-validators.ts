import { z } from "@medusajs/framework/zod"

export const DevicePhotoUpload = z.strictObject({
  files: z.array(z.strictObject({
    filename: z.string().min(1).max(200),
    mime_type: z.enum(["image/jpeg", "image/png", "image/webp"]),
    content: z.string().min(1).max(4194304),
  })).min(1).max(8),
})
export type DevicePhotoUploadBody = z.infer<typeof DevicePhotoUpload>
