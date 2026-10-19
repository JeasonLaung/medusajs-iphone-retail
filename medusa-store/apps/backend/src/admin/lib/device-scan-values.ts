import { isImeiOrEid } from "../../lib/device-identity"

export function classifyDeviceScans(values: string[]) {
  const serials = new Set<string>()
  const radios = new Set<string>()
  const unknown = new Set<string>()
  for (const raw of values) {
    for (const part of raw.split(/[\r\n;,|]+/)) {
      const value = part.trim().toUpperCase().replace(
        /^(?:S\/?N|SERIAL(?:\s*NUMBER)?|IMEI\s*[12]?|EID\s*[12]?|序列号)\s*[:：=]\s*/, "",
      )
      if (!value) continue
      if (isImeiOrEid(value)) radios.add(value)
      else if (/^[A-Z0-9-]{1,100}$/.test(value) && (!/^\d+$/.test(value) || value.length < 15)) serials.add(value)
      else unknown.add(value)
    }
  }
  return { serials: [...serials], radios: [...radios], unknown: [...unknown] }
}

export function mergeDeviceScans(current: { serial: string; imei: string; imei2: string }, values: string[]) {
  const parsed = classifyDeviceScans(values)
  const next = { ...current }
  let conflict = parsed.serials.length > 1 || parsed.unknown.length > 0
  if (parsed.serials.length === 1) {
    if (!next.serial.trim()) next.serial = parsed.serials[0]
    else if (next.serial.trim().toUpperCase() !== parsed.serials[0]) conflict = true
  }
  for (const value of parsed.radios) {
    if ([next.imei.trim(), next.imei2.trim()].includes(value)) continue
    if (!next.imei.trim()) next.imei = value
    else if (!next.imei2.trim()) next.imei2 = value
    else conflict = true
  }
  return { ...next, conflict }
}
