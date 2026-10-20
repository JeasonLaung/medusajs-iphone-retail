import { z } from "@medusajs/framework/zod"
import { DeviceCategory, IntakeDevice } from "../admin/devices/validators"

export const CreateMobileIntake = z.strictObject({ source_id: z.string().trim().min(1).max(100), category: DeviceCategory.default("sale") })
export type CreateMobileIntakeBody = z.infer<typeof CreateMobileIntake>
const { source_id: sourceId, category: category, ...mobileFields } = IntakeDevice.shape
// The warehouse is taken exclusively from the signed session.
export const MobileIntakeBody = z.strictObject(mobileFields).refine((device) => device.serial_number, "请填写设备序列号")
export type MobileIntakeBodyType = z.infer<typeof MobileIntakeBody>
