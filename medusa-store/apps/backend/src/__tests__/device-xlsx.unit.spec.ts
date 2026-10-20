import ExcelJS from "exceljs"
import { parseDeviceFile } from "../admin/lib/device-xlsx"
import { createDeviceTemplate, templateSkuSearchText } from "../lib/device-import-template"
import { templateSkuParameters } from "../lib/device-import-columns"

const message = (key: string) => key
const file = (buffer: ExcelJS.Buffer, name = "devices.xlsx") => ({
  name,
  arrayBuffer: async () => buffer,
}) as unknown as File

describe("device XLSX import", () => {
  it("indexes model, storage, grade and battery as separate exact-match terms", () => {
    const text = templateSkuSearchText({ sku: "CODE", product: { title: "iPhone 14" }, title: "Black / 256 GB / A / 85%", options: [
      { value: "256 GB", option: { title: "Storage" } },
      { value: "A", option: { title: "Condition" } },
      { value: "85%", option: { title: "Battery" } },
    ] })
    for (const term of ["iphone14", "256gb", "gradea", "a", "85%", "iphone14256gbgradea85%", "iphone14256gba85%", "256gba"]) expect(text).toContain(`|${term}|`)
    expect(text).not.toContain("|iphone14128gba85%|")
    expect(text).not.toContain("|iphone14256gbgradeb85%|")
    expect(templateSkuSearchText({ product: { title: "iPhone 14 Pro" }, title: "Black / 256 GB / A / 85%" })).not.toContain("|iphone14256gba85%|")
    expect(templateSkuSearchText({ product: { title: "iPhone 14 Pro" }, title: "Black / B / 100%" })).not.toContain("|iphone14|")
    expect(templateSkuSearchText({ title: "Black / B / 100%" })).not.toContain("|gradea|")
  })
  it("uses the complete SKU source as a named dropdown range for all 200 device rows", async () => {
    const variants = Array.from({ length: 1100 }, (_, i) => ({ sku: `SKU-${i}`, title: `Variant ${i}`, product: { title: "iPhone" } }))
    const workbook = new ExcelJS.Workbook()
    await workbook.xlsx.load(await createDeviceTemplate([...variants, variants[0], { sku: null }]))
    const sheet = workbook.worksheets[0]
    const source = workbook.getWorksheet("SKU source")!
    expect(source.rowCount).toBe(1101)
    expect(workbook.worksheets.filter((sheet) => sheet.state === "visible").map((sheet) => sheet.name)).toEqual(["Devices", "SKU source"])
    expect(workbook.getWorksheet("SKU search")).toBeUndefined()
    expect(source.autoFilter).toBe("A1:F1101")
    expect(source.getCell("E2").text).toContain("iphone")
    expect(source.getCell("A1101").text).toBe("SKU-1099")
    expect(source.getCell("F1101").text).toBe("iPhone/Variant1099/[SKU-1099]")
    expect(workbook.definedNames.getRanges("DeviceSkuParameters").ranges).toEqual(["'SKU source'!$F$2:$F$1101"])
    expect(workbook.definedNames.getRanges("DeviceSkuCodes").ranges).toEqual(["'SKU source'!$A$2:$A$1101"])
    expect(workbook.getWorksheet("SKU choices")?.state).toBe("veryHidden")
    expect(workbook.getWorksheet("SKU choices")?.getCell("A1").formula).toContain("Devices!E2")
    expect(workbook.getWorksheet("SKU choices")?.getCell("GR1").formula).toContain("Devices!E201")
    expect(workbook.getWorksheet("SKU choices")?.getCell("A3").formula).toContain('SUBSTITUTE(A$2," ","")')
    for (const cell of ["E2", "E100", "E201"]) {
      expect(sheet.getCell(cell).dataValidation.formulae[0]).toContain("SKU choices")
      expect(sheet.getCell(cell).dataValidation.showErrorMessage).not.toBe(true)
    }
    for (const cell of ["E2", "E100", "E201"]) expect(sheet.getCell(cell).dataValidation).toMatchObject({ type: "list", allowBlank: true, errorStyle: "stop" })
    sheet.getCell("A2").value = "ABC123"
    sheet.getCell("E2").value = source.getCell("F1101").text
    const rows = await parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)
    expect(rows).toHaveLength(1)
    expect(rows[0].sku).toBe("SKU-1099")
  })
  it("downloads a real blank workbook with text identifiers and accepts an optional SKU", async () => {
    const workbook = new ExcelJS.Workbook()
    await workbook.xlsx.load(await createDeviceTemplate())
    const sheet = workbook.worksheets[0]
    expect(sheet.getRow(1).values).toEqual([undefined, "Serial Number", "IMEI", "IMEI 2", "EID", "SKU Parameters", "SKU", "Grade", "Battery Health (%)", "Cost (EUR)", "Currency", "Storage Bin", "Notes"])
    for (const column of [1, 2, 3, 4, 6]) expect(sheet.getColumn(column).numFmt).toBe("@")
    for (const [address, value] of Object.entries({ A2: "000123ABC", B2: "012345678901234", C2: "112345678901234", D2: "89049032000000000000000000000001", I2: 123.45 })) sheet.getCell(address).value = value
    const rows = await parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)
    expect(rows[0]).toMatchObject({ serial_number: "000123ABC", imei: "012345678901234", imei2: "112345678901234", eid: "89049032000000000000000000000001", sku: "", cost_eur: "123.45" })
  })

  it("accepts a workbook without a SKU column", async () => {
    const workbook = new ExcelJS.Workbook()
    workbook.addWorksheet("Devices").addRows([["serial_number", "imei"], ["ABC123", "012345678901234"]])
    expect(await parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)).toEqual([{ serial_number: "ABC123", imei: "012345678901234" }])
  })

  it("imports Chinese template headers and styles all visible table headers in black on white", async () => {
    const workbook = new ExcelJS.Workbook()
    await workbook.xlsx.load(await createDeviceTemplate([], undefined, "zhCN"))
    const sheet = workbook.worksheets[0]
    expect(sheet.getCell("A1").text).toBe("序列号")
    expect(sheet.getCell("E1").text).toBe("SKU 参数")
    expect(sheet.getCell("H1").text).toBe("电池健康度（%）")
    expect(sheet.getCell("I1").text).toBe("采购成本（EUR）")
    expect(workbook.getWorksheet("SKU source")!.getCell("B1").text).toBe("商品")
    expect(workbook.getWorksheet("SKU source")!.getCell("C1").text).toBe("规格")
    for (const [name, address] of [["Devices", "A1"], ["SKU source", "B1"]]) {
      const cell = workbook.getWorksheet(name)!.getCell(address)
      expect(cell.font).toMatchObject({ bold: true, size: 11, color: { argb: "FF000000" } })
      expect(cell.fill).toMatchObject({ fgColor: { argb: "FFFFFFFF" } })
      expect(cell.alignment).toMatchObject({ vertical: "middle", wrapText: true })
    }
    for (const [address, value] of Object.entries({ A2: "000123ABC", B2: "012345678901234", G2: "A", H2: 85, I2: 123.45, J2: "eur", K2: "A-01", L2: "备注内容" })) sheet.getCell(address).value = value
    expect((await parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message))[0]).toMatchObject({ serial_number: "000123ABC", sku: "", grade: "A", battery_health: "85", cost_eur: "123.45", currency_code: "eur", bin: "A-01", notes: "备注内容" })
  })

  it("locks automatic SKU cells while leaving identifiers and parameters editable", async () => {
    const workbook = new ExcelJS.Workbook()
    await workbook.xlsx.load(await createDeviceTemplate())
    const sheet = workbook.worksheets[0]
    expect((sheet.model as any).sheetProtection.sheet).toBe(true)
    for (const row of [2, 100, 201]) {
      expect(sheet.getCell(`E${row}`).protection.locked).toBe(false)
      expect(sheet.getCell(`A${row}`).protection.locked).toBe(false)
      expect(sheet.getCell(`L${row}`).protection.locked).toBe(false)
      expect(sheet.getCell(`F${row}`).protection?.locked ?? true).toBe(true)
      expect(sheet.getCell(`F${row}`).formula).toContain(`MATCH(SUBSTITUTE`)
    }
    await expect(parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)).rejects.toThrow("deviceScan.csvRows")
  })

  it("displays actual options, resolves selections without cached results, and ignores stale cached SKUs", async () => {
    const variant = { sku: "REAL-CODE", product: { title: "iPhone 14" }, title: "Outdated title", options: [{ value: "256 GB" }, { value: "Black" }, { value: "A" }, { value: "85%" }] }
    const workbook = new ExcelJS.Workbook()
    await workbook.xlsx.load(await createDeviceTemplate([variant]))
    const sheet = workbook.worksheets[0]
    expect(templateSkuParameters(variant)).toBe("iPhone14/256GB/Black/A/85%/[REAL-CODE]")
    sheet.getCell("A2").value = "ABC"
    sheet.getCell("E2").value = templateSkuParameters(variant)
    sheet.getCell("F2").value = { formula: sheet.getCell("F2").formula, result: "WRONG-CACHED-SKU" }
    expect((await parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message))[0].sku).toBe("REAL-CODE")
    sheet.getCell("E2").value = "iphone14 256gb"
    await expect(parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)).rejects.toThrow("deviceScan.importSkuParameters")
    sheet.getCell("E2").value = templateSkuParameters(variant)
    sheet.getCell("F2").value = "OTHER-SKU"
    await expect(parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)).rejects.toThrow("deviceScan.importSkuParameters")
    sheet.getCell("F2").value = { formula: 'HYPERLINK("https://example.com","REAL-CODE")', result: "REAL-CODE" }
    await expect(parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)).rejects.toThrow("deviceScan.importFormula")
  })

  it("matches mixed languages, legacy fields and common English header formatting", async () => {
    const workbook = new ExcelJS.Workbook()
    workbook.addWorksheet("Devices").addRows([
      [" Serial\nNumber ", "IMEI 2", "成色", "battery_health", "Purchase Cost (EUR)", "Currency Code", "Bin Location", "Remarks", "Variant ID"],
      ["000ABC", "012345678901234", "B", 90, 100, "eur", "B1", "note", "variant_123"],
    ])
    expect(await parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)).toEqual([
      { serial_number: "000ABC", imei2: "012345678901234", grade: "B", battery_health: "90", cost_eur: "100", currency_code: "eur", bin: "B1", notes: "note", variant_id: "variant_123" },
    ])
  })

  it.each([["序列号", "Serial Number"], ["serial_number", "Serial Number"], ["Serial Number", "Unknown Header"]])("rejects duplicate field mappings or unknown headers: %s, %s", async (...headers) => {
    const workbook = new ExcelJS.Workbook()
    workbook.addWorksheet("Devices").addRows([headers, ["ABC", "DEF"]])
    await expect(parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)).rejects.toThrow("deviceScan.csvHeaders")
  })

  it("rejects formulas and extra cells instead of silently importing their contents", async () => {
    const workbook = new ExcelJS.Workbook()
    const sheet = workbook.addWorksheet("Devices")
    sheet.addRows([["serial_number"], [{ formula: '"ABC123"', result: "ABC123" }]])
    await expect(parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)).rejects.toThrow("deviceScan.importFormula")
    sheet.getCell("A2").value = "ABC123"
    sheet.getCell("B2").value = "unmapped"
    await expect(parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)).rejects.toThrow("deviceScan.csvColumns")
  })

  it("rejects CSV uploads", async () => {
    await expect(parseDeviceFile(file(new ArrayBuffer(0), "devices.csv"), message)).rejects.toThrow("deviceScan.importFileType")
  })
})
