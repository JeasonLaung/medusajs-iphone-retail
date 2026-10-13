// RFC-style quoted fields, escaped quotes, CRLF, BOM and multiline cells.
export function parseDeviceCsv(text: string): Record<string, string>[] {
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
      else throw new Error("CSV 引号格式错误")
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
  if (quoted) throw new Error("CSV 引号未闭合")
  row.push(field)
  if (row.some((s) => s.trim())) rows.push(row)
  const headers = rows.shift()?.map((s) => s.trim().toLowerCase()) || []
  if (!headers.length || new Set(headers).size !== headers.length)
    throw new Error("CSV 表头缺失或重复")
  if (!headers.includes("serial_number") && !headers.includes("imei"))
    throw new Error(
      "CSV 需要 serial_number 或 imei 列；在 Excel 中请将号码设为文本格式",
    )
  if (rows.length > 200 || rows.length === 0)
    throw new Error("每批请导入 1–200 台设备")
  return rows.map((values, index) => {
    if (values.length !== headers.length)
      throw new Error(`CSV 第 ${index + 2} 行列数不正确`)
    return Object.fromEntries(headers.map((h, i) => [h, values[i].trim()]))
  })
}
