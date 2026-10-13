import { MedusaError } from "@medusajs/framework/utils"

export function requireDevice(condition: unknown, message: string): asserts condition {
  if (!condition) throw new MedusaError(MedusaError.Types.INVALID_DATA, message)
}

export function matchesIdentity(scan: string, device: { serial_number?: string | null; imei?: string | null }) {
  const normalized = scan.trim().toUpperCase()
  return normalized === device.serial_number || normalized === device.imei
}

export function assertVariantConfiguration(variant: { options?: { value?: string; option?: { title?: string } | null }[] }, device: { grade: string; battery_health: number }) {
  for (const option of variant.options || []) {
    const title = option.option?.title?.toLowerCase() || ""
    const value = option.value || ""
    if (/grade|condition|成色/.test(title)) requireDevice(value.toUpperCase() === device.grade, "设备成色与 SKU 不匹配，请选择正确的 SKU")
    if (/battery|电池/.test(title)) {
      const minimum = Number(value.replace(/[^0-9.]/g, ""))
      if (minimum > 0) requireDevice(device.battery_health >= minimum, "实际电池健康度低于 SKU 电池档位")
    }
  }
}
