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
  return messages.reduce(
    (result, entry) => result.split(entry.message).join(translate(entry.key)),
    message,
  )
}
