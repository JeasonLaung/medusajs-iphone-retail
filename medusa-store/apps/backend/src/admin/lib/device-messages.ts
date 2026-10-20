import en from "../i18n/json/en.json"
import zhCN from "../i18n/json/zh-CN.json"

// Localize known business errors without changing API payloads or stored records.
const messages = Object.entries(zhCN.deviceScan.errors).flatMap(
  ([key, message]) => [
    { key: `deviceScan.errors.${key}`, message },
    {
      key: `deviceScan.errors.${key}`,
      message: en.deviceScan.errors[key as keyof typeof en.deviceScan.errors],
    },
  ],
)

export function localizeDeviceMessage(
  message: string,
  translate: (key: string) => string,
) {
  const codes: Record<string, string> = { DEVICE_ALLOCATION_LOCKED: "allocationLocked", DEVICE_FULFILLMENT_LOCATION: "fulfillmentLocation", DEVICE_INTAKE_REQUIRED: "intakeRequired", DEVICE_INSPECTION_CATEGORY: "inspectionCategory", DEVICE_LOCKED: "deviceLocked", DEVICE_HAS_SALES_HISTORY: "deviceSaleHistory", DEVICE_PHOTO_INVALID: "photoInvalid", DEVICE_EDIT_ALLOCATED: "editAllocated", DEVICE_EDIT_STATE: "editState", DEVICE_EDIT_CATEGORY: "editCategory", DEVICE_EDIT_LOCATION: "editLocation" }
  for (const [code, key] of Object.entries(codes)) message = message.split(code).join(translate(`deviceScan.errors.${key}`))
  return messages.reduce(
    (result, entry) => result.split(entry.message).join(translate(entry.key)),
    message,
  )
}
