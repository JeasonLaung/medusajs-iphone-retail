import en from "../i18n/json/en.json"

// RFC-style quoted fields, escaped quotes, CRLF, BOM and multiline cells.
export function parseDeviceCsv(
  text: string,
  message: (key: string, values?: { row: number }) => string = (
    key,
    values,
  ) => {
    const template =
      en.deviceScan[
        key.replace("deviceScan.", "") as keyof typeof en.deviceScan
      ]
    return typeof template === "string"
      ? template.replace("{{row}}", String(values?.row ?? ""))
      : key
  },
): Record<string, string>[] {
  const rows: string[][] = []
  let row: string[] = [],
    field = "",
    quoted = false
  const input = text.replace(/^\uFEFF/, "")
  for (let i = 0; i < input.length; i++) {
    const c = input[i]
    if (c === '"') {
      if (quoted && input[i + 1] === '"') {
        field += '"'
        i += 1
      } else if (quoted || field.length === 0) quoted = !quoted
      else throw new Error(message("deviceScan.csvQuotes"))
    } else if (c === "," && !quoted) {
      row.push(field)
      field = ""
    } else if ((c === "\n" || c === "\r") && !quoted) {
      if (c === "\r" && input[i + 1] === "\n") i += 1
      row.push(field)
      if (row.some((s) => s.trim())) rows.push(row)
      row = []
      field = ""
    } else field += c
  }
  if (quoted) throw new Error(message("deviceScan.csvUnclosed"))
  row.push(field)
  if (row.some((s) => s.trim())) rows.push(row)
  const headers = rows.shift()?.map((s) => s.trim().toLowerCase()) || []
  if (!headers.length || new Set(headers).size !== headers.length)
    throw new Error(message("deviceScan.csvHeaders"))
  if (!headers.includes("serial_number") && !headers.includes("imei"))
    throw new Error(message("deviceScan.csvIdentity"))
  if (rows.length > 200 || rows.length === 0)
    throw new Error(message("deviceScan.csvRows"))
  return rows.map((values, index) => {
    if (values.length !== headers.length)
      throw new Error(message("deviceScan.csvColumns", { row: index + 2 }))
    return Object.fromEntries(headers.map((h, i) => [h, values[i].trim()]))
  })
}
