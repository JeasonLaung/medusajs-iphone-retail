import type { Workbook } from "exceljs"
import { matchDeviceImportHeader, templateSkuFormula } from "../../lib/device-import-columns"

async function newWorkbook(): Promise<Workbook> {
  const { default: ExcelJS } = await import("exceljs")
  return new ExcelJS.Workbook()
}

export async function buildDeviceExport(
  columns: { header: string; key: string; width?: number; numFmt?: string }[],
  rows: Record<string, string | number | null>[],
) {
  const workbook = await newWorkbook()
  const sheet = workbook.addWorksheet("Devices")
  sheet.columns = columns.map((column) => ({ width: 24, ...column }))
  sheet.views = [{ state: "frozen", ySplit: 1 }]
  sheet.getRow(1).font = { bold: true }
  for (const row of rows) sheet.addRow(row)
  sheet.autoFilter = { from: { row: 1, column: 1 }, to: { row: Math.max(1, sheet.rowCount), column: columns.length } }
  return workbook.xlsx.writeBuffer()
}

export async function parseDeviceFile(file: File, message: (key: string, values?: { row: number }) => string) {
  if (!/\.xlsx$/i.test(file.name)) throw new Error(message("deviceScan.importFileType"))
  const workbook = await newWorkbook()
  await workbook.xlsx.load(await file.arrayBuffer())
  const sheet = workbook.worksheets[0]
  if (!sheet) throw new Error(message("deviceScan.csvHeaders"))
  if (sheet.rowCount > 201) throw new Error(message("deviceScan.csvRows"))
  const width = sheet.getRow(1).cellCount
  const matchedHeaders = Array.from({ length: width }, (_, i) => matchDeviceImportHeader(sheet.getRow(1).getCell(i + 1).text))
  if (!matchedHeaders.length || matchedHeaders.some((header) => !header) || new Set(matchedHeaders).size !== matchedHeaders.length) throw new Error(message("deviceScan.csvHeaders"))
  const headers = matchedHeaders as string[]
  if (!headers.includes("serial_number") && !headers.includes("imei") && !headers.includes("imei2") && !headers.includes("eid")) throw new Error(message("deviceScan.csvIdentity"))
  const rows: Record<string, string>[] = []
  const source = workbook.getWorksheet("SKU source")
  const parameterSkus = new Map<string, Set<string>>()
  source?.eachRow((row) => {
    if (row.number === 1) return
    const sku = row.getCell(1)
    const parameters = row.getCell(6)
    if (sku.type === 6 || parameters.type === 6) return
    if (!sku.text.trim() || !parameters.text.trim()) return
    const key = parameters.text.trim().toLowerCase()
    const matches = parameterSkus.get(key) || new Set<string>()
    matches.add(sku.text.trim())
    parameterSkus.set(key, matches)
  })
  const parameterIndex = headers.indexOf("sku_parameters")
  const parameterColumn = sheet.getColumn(parameterIndex + 1 || 1).letter
  sheet.eachRow((row) => {
    if (row.number === 1) return
    const values = Array.from({ length: Math.max(sheet.getRow(1).cellCount, row.cellCount) }, (_, i) => {
      const cell = row.getCell(i + 1)
      if (cell.type === 6) {
        if (headers[i] !== "sku" || parameterIndex < 0 || !source || cell.formula !== templateSkuFormula(row.number, Math.max(2, source.rowCount), parameterColumn)) throw new Error(message("deviceScan.importFormula"))
        // Resolve the selection independently; cached formula values can be stale or missing.
        return ""
      }
      return cell.text
    })
    if (values.slice(width).some((value) => value.trim())) throw new Error(message("deviceScan.csvColumns", { row: row.number }))
    if (!values.some((value) => value.trim())) return
    const record = Object.fromEntries(headers.map((header, i) => [header, values[i].trim()]))
    if (record.sku_parameters) {
      const matches = parameterSkus.get(record.sku_parameters.toLowerCase())
      if (matches?.size !== 1) throw new Error(message("deviceScan.importSkuParameters", { row: row.number }))
      const sku = [...matches][0]
      if (record.sku && record.sku !== sku) throw new Error(message("deviceScan.importSkuParameters", { row: row.number }))
      record.sku = sku
    }
    rows.push(record)
  })
  if (!rows.length || rows.length > 200) throw new Error(message("deviceScan.csvRows"))
  return rows
}
