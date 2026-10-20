import ExcelJS from "exceljs"
import { deviceImportHeaders, deviceImportKeys, templateSkuParameters, templateSkuFormula, type TemplateSku } from "./device-import-columns"

export type { TemplateSku } from "./device-import-columns"

export function templateSkuSearchText(variant: TemplateSku) {
  const compact = (value: string) => value.normalize("NFKC").toLowerCase().replace(/\s+/g, "")
  const values = [variant.sku || "", variant.product?.title || "", variant.title || ""]
  for (const option of variant.options || []) {
    values.push(option.value)
    if (/condition|grade|成色/i.test(option.option?.title || "")) values.push(`grade${option.value}`)
  }
  // Legacy titles may contain a grade without named product options.
  const grade = variant.title?.match(/(?:^|[\s/|·-])(?:grade\s*)?([sabcd])(?=$|[\s/|·-])/i)?.[1]
  if (grade) values.push(grade, `grade${grade}`)
  // Index ordered combinations so model/storage/grade/battery can be typed together.
  const parts = [variant.product?.title || "", ...(variant.options?.length
    ? variant.options.map((option) => option.value)
    : (variant.title || "").split(/[/|·]/))].map(compact).filter(Boolean)
  let combinations = [""]
  for (const part of parts.slice(0, 8)) {
    const alternatives = /^[sabcd]$/.test(part) ? [part, `grade${part}`] : [part]
    combinations = [...combinations, ...combinations.flatMap((prefix) => alternatives.map((value) => prefix + value))]
  }
  let length = values.map(compact).join("|").length + 2
  for (const value of new Set(combinations.filter(Boolean))) {
    if (length + value.length + 1 > 32000) break
    values.push(value)
    length += value.length + 1
  }
  return `|${values.map(compact).join("|")}|`
}
type TemplateMessages = { skuPrompt: string; skuError: string }

function normalizeQuery(input: string) {
  let formula = `LOWER(TRIM(SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(${input},CHAR(160)," "),CHAR(10)," "),CHAR(9)," ")))`
  for (const [from, to] of [["iphone ", "iphone"], ["grade ", "grade"], [" gb", "gb"], [" tb", "tb"], [" %", "%"], [" pro max", "promax"], [" pro", "pro"], [" plus", "plus"], [" mini", "mini"]]) {
    formula = `SUBSTITUTE(${formula},"${from}","${to}")`
  }
  return formula
}

function queryPattern(token: string) {
  const exact = `OR(AND(LEFT(${token},6)="iphone",LEN(${token})>6),LEFT(${token},5)="grade",RIGHT(${token},2)="gb",RIGHT(${token},2)="tb",RIGHT(${token},1)="%",AND(LEN(${token})=1,ISNUMBER(SEARCH(${token},"sabcd"))))`
  const escaped = `SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(${token},"~","~~"),"*","~*"),"?","~?")`
  return `IF(${exact},"|"&${escaped}&"|",${escaped})`
}

function columnName(index: number): string {
  return index > 26 ? columnName(Math.floor((index - 1) / 26)) + columnName((index - 1) % 26 + 1) : String.fromCharCode(64 + index)
}

function addInlineSkuSearch(workbook: ExcelJS.Workbook, devices: ExcelJS.Worksheet, lastRow: number) {
  const choices = workbook.addWorksheet("SKU choices", { state: "veryHidden" })
  const source = `'SKU source'!$F$2:$F$${lastRow}`
  const text = `'SKU source'!$E$2:$E$${lastRow}`
  for (let row = 2; row <= 201; row++) {
    const column = columnName(row - 1)
    choices.getCell(`${column}1`).value = { formula: `IF(Devices!E${row}="","",Devices!E${row})`, result: "" }
    choices.getCell(`${column}2`).value = { formula: normalizeQuery(`${column}1`), result: "" }
    const factors: string[] = []
    for (let term = 0; term < 12; term++) {
      const token = `${column}${term + 4}`
      const pattern = `${column}${term + 16}`
      choices.getCell(token).value = { formula: `TRIM(MID(SUBSTITUTE(${column}$2," ",REPT(" ",200)),${term * 200 + 1},200))`, result: "" }
      choices.getCell(pattern).value = { formula: queryPattern(token), result: "" }
      factors.push(`--ISNUMBER(SEARCH(${pattern},${text}))`)
    }
    const compactQuery = `SUBSTITUTE(${column}$2," ","")`
    const compactMatch = `ISNUMBER(SEARCH("|"&SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(${compactQuery},"~","~~"),"*","~*"),"?","~?")&"|",${text}))`
    const include = `(${source}<>"")*--(((${source}=${column}$1)+(${factors.join("*")})+(${compactMatch}))>0)`
    choices.getCell(`${column}3`).value = {
      formula: `IF(${column}1="",0,SUMPRODUCT(${include}))`, result: 0,
    }
    // A fixed array with a count mask works in WPS without requiring spill references.
    const arrayFormula: ExcelJS.CellFormulaValue & { shareType: "array"; ref: string } = {
      formula: `IF(ROW($A$1:$A$${lastRow - 1})<=${column}$3,IFERROR(_xlfn._xlws.FILTER(${source},${include},""),""),"")`,
      shareType: "array", ref: `${column}30:${column}${lastRow + 28}`, result: "",
    }
    choices.getCell(`${column}30`).value = arrayFormula
    const validation = devices.getCell(row, 5).dataValidation
    validation.formulae = [`IF($E${row}="",DeviceSkuParameters,INDIRECT("'SKU choices'!$${column}$30:$${column}$"&MAX(30,29+'SKU choices'!$${column}$3)))`]
    // Allow temporary search terms; import requires a complete parameter selection.
    validation.showErrorMessage = false
  }
}

function styleTemplateHeader(sheet: ExcelJS.Worksheet, rowNumber: number, columnCount: number) {
  const row = sheet.getRow(rowNumber)
  row.height = 36
  for (let column = 1; column <= columnCount; column++) {
    const cell = row.getCell(column)
    cell.font = { name: "Calibri", size: 11, bold: true, color: { argb: "FF000000" } }
    cell.fill = { type: "pattern", pattern: "solid", fgColor: { argb: "FFFFFFFF" } }
    cell.alignment = { vertical: "middle", horizontal: "left", indent: 1, wrapText: true }
    cell.border = { bottom: { style: "thin", color: { argb: "FF000000" } } }
  }
}

export async function createDeviceTemplate(variants: TemplateSku[] = [], messages?: TemplateMessages, locale = "en") {
  const workbook = new ExcelJS.Workbook()
  workbook.calcProperties.fullCalcOnLoad = true
  const sheet = workbook.addWorksheet("Devices", { views: [{ state: "frozen", ySplit: 1 }] })
  const headers = deviceImportHeaders(locale)
  const widths: Partial<Record<typeof deviceImportKeys[number], number>> = { serial_number: 24, imei: 22, imei2: 22, eid: 38, sku_parameters: 72, sku: 32, grade: 16, battery_health: 26, notes: 44 }
  sheet.columns = deviceImportKeys.map((key) => ({ header: headers[key], key, width: widths[key] || 24 }))
  styleTemplateHeader(sheet, 1, deviceImportKeys.length)
  for (const name of ["serial_number", "imei", "imei2", "eid", "sku"]) sheet.getColumn(name).numFmt = "@"

  const source = workbook.addWorksheet("SKU source", { views: [{ state: "frozen", ySplit: 1 }] })
  source.columns = [
    { header: headers.sku, key: "sku", width: 28 },
    { header: headers.product, key: "product", width: 34 },
    { header: headers.variant, key: "variant", width: 60 },
    { key: "match_order", width: 12, hidden: true },
    { key: "search_index", width: 12, hidden: true },
    { header: headers.sku_parameters, key: "sku_parameters", width: 72 },
  ]
  source.getColumn("sku").numFmt = "@"
  styleTemplateHeader(source, 1, 6)
  const unique = new Map(variants.filter((variant) => variant.sku?.trim()).map((variant) => [variant.sku!, variant]))
  for (const [sku, variant] of unique) {
    source.addRow([sku, variant.product?.title || "", variant.title || "", "", templateSkuSearchText(variant), templateSkuParameters(variant)])
  }
  const lastSourceRow = Math.max(2, source.rowCount)
  source.autoFilter = { from: "A1", to: `F${lastSourceRow}` }
  // A named range avoids Excel's 255-character limit for inline lists.
  workbook.definedNames.add(`'SKU source'!$A$2:$A$${Math.max(2, source.rowCount)}`, "DeviceSkuCodes")
  workbook.definedNames.add(`'SKU source'!$F$2:$F$${lastSourceRow}`, "DeviceSkuParameters")
  for (let row = 2; row <= 201; row++) {
    for (let column = 1; column <= deviceImportKeys.length; column++) {
      sheet.getCell(row, column).protection = { locked: column === 6 }
    }
    sheet.getCell(row, 6).value = { formula: templateSkuFormula(row, lastSourceRow), result: "" }
    sheet.getCell(row, 5).dataValidation = {
      type: "list",
      allowBlank: true,
      formulae: ["DeviceSkuParameters"],
      showInputMessage: !!messages,
      prompt: messages?.skuPrompt,
      showErrorMessage: true,
      errorStyle: "stop",
      error: messages?.skuError,
    }
  }
  addInlineSkuSearch(workbook, sheet, lastSourceRow)
  await sheet.protect("", { selectLockedCells: true, selectUnlockedCells: true })
  await source.protect("", { autoFilter: true })
  return workbook.xlsx.writeBuffer()
}
