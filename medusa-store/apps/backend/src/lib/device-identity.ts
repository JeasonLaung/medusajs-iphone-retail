// Keep identifiers as strings: EIDs exceed JavaScript's safe integer range.
export function isImeiOrEid(value: string) {
  return /^(?:\d{15}|\d{32})$/.test(value.trim())
}

export type DeviceIdentity = {
  serial_number?: string | null
  imei?: string | null
  imei2?: string | null
}

export function deviceRadioValues(device: DeviceIdentity) {
  return [device.imei?.trim(), device.imei2?.trim()].filter((value): value is string => !!value)
}

export function deviceIdentityFilters(device: DeviceIdentity) {
  return [
    ...(device.serial_number ? [{ serial_number: device.serial_number }] : []),
    ...deviceRadioValues(device).flatMap((value) => [{ imei: value }, { imei2: value }]),
  ]
}

export function matchesDeviceIdentity(scan: string, device: DeviceIdentity) {
  const normalized = scan.trim().toUpperCase()
  return !!normalized && [device.serial_number, device.imei, device.imei2].some(
    (value) => value?.trim().toUpperCase() === normalized,
  )
}
