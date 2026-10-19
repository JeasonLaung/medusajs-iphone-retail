import type { Workbook } from "exceljs"

async function newWorkbook(): Promise<Workbook> {
  const { default: ExcelJS } = await import("exceljs")
  return new ExcelJS.Workbook()
}

export async function parseDeviceFile(file: File, message: (key: string, values?: { row: number }) => string) {
  if (!/\.xlsx$/i.test(file.name)) throw new Error(message("deviceScan.importFileType"))
  const workbook = await newWorkbook()
  await workbook.xlsx.load(await file.arrayBuffer())
  const sheet = workbook.worksheets[0]
  if (!sheet) throw new Error(message("deviceScan.csvHeaders"))
  if (sheet.rowCount > 201) throw new Error(message("deviceScan.csvRows"))
  const width = sheet.getRow(1).cellCount
  const headers = Array.from({ length: width }, (_, i) => sheet.getRow(1).getCell(i + 1).text.trim().toLowerCase())
  if (!headers.length || headers.some((header) => !header) || new Set(headers).size !== headers.length) throw new Error(message("deviceScan.csvHeaders"))
  if (!headers.includes("serial_number") && !headers.includes("imei") && !headers.includes("imei2")) throw new Error(message("deviceScan.csvIdentity"))
  const rows: Record<string, string>[] = []
  sheet.eachRow((row) => {
    if (row.number === 1) return
    const values = Array.from({ length: Math.max(sheet.getRow(1).cellCount, row.cellCount) }, (_, i) => {
      const cell = row.getCell(i + 1)
      if (cell.type === 6) throw new Error(message("deviceScan.importFormula"))
      return cell.text
    })
    if (values.slice(width).some((value) => value.trim())) throw new Error(message("deviceScan.csvColumns", { row: row.number }))
    if (values.some((value) => value.trim())) rows.push(Object.fromEntries(headers.map((header, i) => [header, values[i].trim()])))
  })
  if (!rows.length || rows.length > 200) throw new Error(message("deviceScan.csvRows"))
  return rows
}
