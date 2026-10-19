import ExcelJS from "exceljs"
import { parseDeviceFile } from "../admin/lib/device-xlsx"
import { createDeviceTemplate, templateSkuSearchText } from "../lib/device-import-template"

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
    for (const term of ["iphone14", "256gb", "gradea", "a", "85%"]) expect(text).toContain(`|${term}|`)
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
    expect(source.getCell("A1101").text).toBe("SKU-1099")
    expect(workbook.definedNames.getRanges("DeviceSkuCodes").ranges).toEqual(["'SKU source'!$A$2:$A$1101"])
    expect(workbook.getWorksheet("SKU choices")?.state).toBe("veryHidden")
    expect(workbook.getWorksheet("SKU choices")?.getCell("A1").formula).toContain("Devices!D2")
    expect(workbook.getWorksheet("SKU choices")?.getCell("GR1").formula).toContain("Devices!D201")
    for (const cell of ["D2", "D100", "D201"]) {
      expect(sheet.getCell(cell).dataValidation.formulae[0]).toContain("SKU choices")
      expect(sheet.getCell(cell).dataValidation.showErrorMessage).not.toBe(true)
    }
    for (const cell of ["D2", "D100", "D201"]) expect(sheet.getCell(cell).dataValidation).toMatchObject({ type: "list", allowBlank: true, errorStyle: "stop" })
    sheet.getCell("A2").value = "ABC123"
    sheet.getCell("D2").value = "SKU-1099"
    const rows = await parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)
    expect(rows).toHaveLength(1)
    expect(rows[0].sku).toBe("SKU-1099")
  })
  it("downloads a real blank workbook with text identifiers and accepts an optional SKU", async () => {
    const workbook = new ExcelJS.Workbook()
    await workbook.xlsx.load(await createDeviceTemplate())
    const sheet = workbook.worksheets[0]
    expect(sheet.getRow(1).values).toEqual([undefined, "serial_number", "imei", "imei2", "sku", "grade", "battery_health", "cost_eur", "currency_code", "bin", "notes"])
    for (const column of [1, 2, 3, 4]) expect(sheet.getColumn(column).numFmt).toBe("@")
    sheet.getRow(2).values = ["000123ABC", "012345678901234", "89049032000000000000000000000001", "", "", "", 123.45]
    const rows = await parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)
    expect(rows[0]).toMatchObject({ serial_number: "000123ABC", imei: "012345678901234", imei2: "89049032000000000000000000000001", sku: "", cost_eur: "123.45" })
  })

  it("accepts a workbook without a SKU column", async () => {
    const workbook = new ExcelJS.Workbook()
    workbook.addWorksheet("Devices").addRows([["serial_number", "imei"], ["ABC123", "012345678901234"]])
    expect(await parseDeviceFile(file(await workbook.xlsx.writeBuffer()), message)).toEqual([{ serial_number: "ABC123", imei: "012345678901234" }])
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
