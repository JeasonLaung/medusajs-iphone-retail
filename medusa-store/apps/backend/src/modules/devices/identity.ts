import { requireDevice } from "./policy"
import { deviceRadioValues, isImei, isEid, type DeviceIdentity } from "../../lib/device-identity"

export type DeviceProduct = {
  title?: string | null
  handle?: string | null
  metadata?: Record<string, unknown> | null
  type?: { value?: string | null } | null
}

export function requiresImei(product?: DeviceProduct | null) {
  if (typeof product?.metadata?.requires_imei === "boolean")
    return product.metadata.requires_imei
  if (typeof product?.metadata?.device_type === "string")
    return product.metadata.device_type.toLowerCase() === "iphone"
  return /\biphone\b/i.test(
    [product?.title, product?.handle, product?.type?.value]
      .filter(Boolean)
      .join(" "),
  )
}

export function assertDeviceIdentity(
  device: DeviceIdentity,
  product?: DeviceProduct | null,
) {
  requireDevice(device.serial_number?.trim(), "请填写设备序列号")
  requireDevice(
    !requiresImei(product) || deviceRadioValues(device).length > 0,
    "iPhone 必须记录序列号和至少一个 IMEI / EID",
  )
  requireDevice(
    [device.imei, device.imei2].filter((v): v is string => !!v).every(isImei),
    "IMEI 必须为 15 位数字",
  )
  requireDevice(!device.eid || isEid(device.eid), "EID 必须为 32 位数字")
  const values = deviceRadioValues(device)
  requireDevice(new Set(values).size === values.length, "两个 IMEI 不能相同")
}
