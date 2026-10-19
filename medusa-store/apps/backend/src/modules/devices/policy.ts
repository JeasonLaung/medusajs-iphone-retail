import { MedusaError } from "@medusajs/framework/utils"
import { matchesDeviceIdentity } from "../../lib/device-identity"

export function requireDevice(condition: unknown, message: string): asserts condition {
  if (!condition) throw new MedusaError(MedusaError.Types.INVALID_DATA, message)
}

export const matchesIdentity = matchesDeviceIdentity

type ConfiguredVariant = { options?: { value?: string; option?: { title?: string } | null }[] }
type DeviceConfiguration = { grade?: string | null; battery_health?: number | null }

export function intakeDeviceConfiguration(variant: ConfiguredVariant, device: DeviceConfiguration) {
  let grade: string | null = null
  let battery: number | null = null
  for (const option of variant.options || []) {
    const title = option.option?.title?.toLowerCase() || ""
    const value = option.value?.trim() || ""
    if (/grade|condition|成色/.test(title) && value) grade = value.toUpperCase()
    if (/battery|电池/.test(title) && value) {
      const percentage = value.match(/\d+(?:\.\d+)?/)
      if (percentage && Number(percentage[0]) <= 100) battery = Number(percentage[0])
    }
  }
  return { grade: device.grade ?? grade, battery_health: device.battery_health ?? battery }
}

export function assertVariantConfiguration(variant: ConfiguredVariant, device: DeviceConfiguration) {
  for (const option of variant.options || []) {
    const title = option.option?.title?.toLowerCase() || ""
    const value = option.value || ""
    if (/grade|condition|成色/.test(title)) requireDevice(value.toUpperCase() === device.grade, "设备成色与 SKU 不匹配，请选择正确的 SKU")
    if (/battery|电池/.test(title)) {
      const minimum = Number(value.replace(/[^0-9.]/g, ""))
      if (minimum > 0) requireDevice(device.battery_health != null && device.battery_health >= minimum, "实际电池健康度低于 SKU 电池档位")
    }
  }
}
