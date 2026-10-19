import ExcelJS from "exceljs"

export type TemplateSku = { sku?: string | null; title?: string; product?: { title?: string } | null; options?: { value: string; option?: { title?: string } | null }[] | null }

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
  return `|${values.map(compact).join("|")}|`
}
type TemplateMessages = { skuPrompt: string; skuError: string; searchTitle?: string; searchLabel?: string; searchHelp?: string; searchCount?: string }

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
  const source = `'SKU source'!$A$2:$A$${lastRow}`
  const text = `'SKU source'!$E$2:$E$${lastRow}`
  for (let row = 2; row <= 201; row++) {
    const column = columnName(row - 1)
    choices.getCell(`${column}1`).value = { formula: `IF(Devices!D${row}="","",Devices!D${row})`, result: "" }
    choices.getCell(`${column}2`).value = { formula: normalizeQuery(`${column}1`), result: "" }
    const factors: string[] = []
    for (let term = 0; term < 12; term++) {
      const token = `${column}${term + 4}`
      const pattern = `${column}${term + 16}`
      choices.getCell(token).value = { formula: `TRIM(MID(SUBSTITUTE(${column}$2," ",REPT(" ",200)),${term * 200 + 1},200))`, result: "" }
      choices.getCell(pattern).value = { formula: queryPattern(token), result: "" }
      factors.push(`--ISNUMBER(SEARCH(${pattern},${text}))`)
    }
    const include = `(${source}<>"")*${factors.join("*")}`
    choices.getCell(`${column}3`).value = {
      formula: `IF(OR(${column}1="",LEN(${column}1)>200,LEN(${column}2)-LEN(SUBSTITUTE(${column}2," ",""))>=12),0,SUMPRODUCT(${include}))`, result: 0,
    }
    // A fixed array with a count mask works in WPS without requiring spill references.
    const arrayFormula: ExcelJS.CellFormulaValue & { shareType: "array"; ref: string } = {
      formula: `IF(ROW($A$1:$A$${lastRow - 1})<=${column}$3,IFERROR(_xlfn._xlws.FILTER(${source},${include},""),""),"")`,
      shareType: "array", ref: `${column}30:${column}${lastRow + 28}`, result: "",
    }
    choices.getCell(`${column}30`).value = arrayFormula
    const validation = devices.getCell(row, 4).dataValidation
    validation.formulae = [`IF($D${row}="",DeviceSkuCodes,INDIRECT("'SKU choices'!$${column}$30:$${column}$"&MAX(30,29+'SKU choices'!$${column}$3)))`]
    // Search terms must be accepted temporarily; the import still requires an exact SKU code.
    validation.showErrorMessage = false
  }
}

export async function createDeviceTemplate(variants: TemplateSku[] = [], messages?: TemplateMessages) {
  const workbook = new ExcelJS.Workbook()
  workbook.calcProperties.fullCalcOnLoad = true
  const sheet = workbook.addWorksheet("Devices", { views: [{ state: "frozen", ySplit: 1 }] })
  const headers = ["serial_number", "imei", "imei2", "sku", "grade", "battery_health", "cost_eur", "currency_code", "bin", "notes"]
  sheet.columns = headers.map((key) => ({ header: key, key, width: key === "notes" ? 40 : 22 }))
  sheet.getRow(1).font = { bold: true, color: { argb: "FFFFFFFF" } }
  sheet.getRow(1).fill = { type: "pattern", pattern: "solid", fgColor: { argb: "FF243746" } }
  sheet.getRow(1).height = 24
  for (const name of ["serial_number", "imei", "imei2", "sku"]) sheet.getColumn(name).numFmt = "@"

  const source = workbook.addWorksheet("SKU source", { views: [{ state: "frozen", ySplit: 1 }] })
  source.columns = [
    { header: "sku", key: "sku", width: 26 },
    { header: "product", key: "product", width: 32 },
    { header: "variant", key: "variant", width: 60 },
  ]
  source.getColumn("sku").numFmt = "@"
  source.getRow(1).font = { bold: true }
  const unique = new Map(variants.filter((variant) => variant.sku?.trim()).map((variant) => [variant.sku!, variant]))
  for (const [sku, variant] of unique) {
    source.addRow([sku, variant.product?.title || "", variant.title || "", "", templateSkuSearchText(variant)])
  }
  const lastSourceRow = Math.max(2, source.rowCount)
  const search = workbook.addWorksheet("SKU search", { views: [{ state: "frozen", ySplit: 5 }] })
  search.columns = [{ width: 28 }, { width: 34 }, { width: 60 }]
  search.mergeCells("A1:C1")
  search.getCell("A1").value = messages?.searchTitle || "SKU search"
  search.getCell("A1").font = { bold: true, size: 16 }
  search.getRow(1).height = 30
  search.getCell("A2").value = messages?.searchLabel || "Keyword"
  search.mergeCells("B2:C2")
  search.getCell("B2").numFmt = "@"
  search.getCell("B2").fill = { type: "pattern", pattern: "solid", fgColor: { argb: "FFE8F0FE" } }
  search.getCell("B2").font = { color: { argb: "FF175CD3" } }
  search.getRow(2).height = 28
  search.mergeCells("A3:C3")
  search.getCell("A3").value = messages?.searchHelp || "Enter part of a SKU, product or variant above. Then select a matching SKU in Devices. Clear the keyword to show all SKUs."
  search.getCell("A3").alignment = { wrapText: true, vertical: "middle" }
  search.getRow(3).height = 36
  search.getCell("A4").value = messages?.searchCount || "Matching SKUs"
  search.getCell("B4").value = { formula: `COUNT('SKU source'!$D$2:$D$${lastSourceRow})`, result: unique.size }
  search.getRow(5).values = ["sku", "product", "variant"]
  search.getRow(5).font = { bold: true }
  source.getColumn(4).hidden = true
  source.getColumn(5).hidden = true
  // Normalize common model/option spelling before splitting an AND query.
  search.getCell("E2").value = { formula: normalizeQuery("$B$2"), result: "" }
  for (let i = 0; i < 12; i++) {
    search.getCell(2, i + 6).value = { formula: `TRIM(MID(SUBSTITUTE($E$2," ",REPT(" ",200)),${i * 200 + 1},200))`, result: "" }
    const token = `${String.fromCharCode(70 + i)}2`
    search.getCell(3, i + 6).value = { formula: queryPattern(token), result: "" }
  }
  for (let column = 5; column <= 17; column++) search.getColumn(column).hidden = true
  for (let row = 2; row <= lastSourceRow; row++) {
    const matches = Array.from({ length: 12 }, (_, i) => {
      const column = String.fromCharCode(70 + i)
      return `OR('SKU search'!$${column}$2="",ISNUMBER(SEARCH('SKU search'!$${column}$3,$E${row})))`
    })
    source.getCell(row, 4).value = {
      formula: `IF(A${row}="","",IF(AND(LEN('SKU search'!$B$2)<=200,LEN('SKU search'!$E$2)-LEN(SUBSTITUTE('SKU search'!$E$2," ",""))<12,${matches.join(",")}),COUNT($D$1:D${row - 1})+1,""))`,
      result: unique.size ? row - 1 : "",
    }
    for (const [index, column] of ["A", "B", "C"].entries()) {
      search.getCell(row + 4, index + 1).value = {
        formula: `IFERROR(INDEX('SKU source'!$${column}$2:$${column}$${lastSourceRow},MATCH(ROW()-5,'SKU source'!$D$2:$D$${lastSourceRow},0)),"")`,
        result: source.getCell(row, index + 1).text,
      }
    }
  }
  search.getColumn(1).numFmt = "@"
  // A named range avoids Excel's 255-character limit for inline lists.
  workbook.definedNames.add(`'SKU source'!$A$2:$A$${Math.max(2, source.rowCount)}`, "DeviceSkuCodes")
  for (let row = 2; row <= 201; row++) {
    sheet.getCell(row, 4).dataValidation = {
      type: "list",
      allowBlank: true,
      formulae: [`INDIRECT("'SKU search'!$A$6:$A$"&MAX(6,5+'SKU search'!$B$4))`],
      showInputMessage: !!messages,
      prompt: messages?.skuPrompt,
      showErrorMessage: true,
      errorStyle: "stop",
      error: messages?.skuError,
    }
  }
  addInlineSkuSearch(workbook, sheet, lastSourceRow)
  return workbook.xlsx.writeBuffer()
}
