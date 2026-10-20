import assert from "node:assert/strict"
import type { ConfigModule, ExecArgs, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"
import ExcelJS from "exceljs"
import { parseDeviceFile } from "../admin/lib/device-xlsx"

export default async function checkDeviceImportTemplate({ container }: ExecArgs) {
  assert(process.env.PLAYWRIGHT_MODULE_PATH, "Set PLAYWRIGHT_MODULE_PATH")
  const users: IUserModuleService = container.resolve(Modules.USER)
  const user = (await users.listUsers({})).find((user) => user.metadata?.device_role !== "supplier")
  assert(user, "An admin user is required")
  const config: ConfigModule = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const token = generateJwtToken({ actor_id: user.id, actor_type: "user", auth_identity_id: "template_check", app_metadata: { user_id: user.id } }, { secret: config.projectConfig.http.jwtSecret, expiresIn: "5m" })
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
  const browser = await chromium.launch({ channel: "chrome", headless: true })
  try {
    const page = await browser.newPage()
    const session = await page.request.post("http://127.0.0.1:9001/auth/session", { headers: { authorization: `Bearer ${token}` } })
    assert.equal(session.status(), 200)
    await page.goto("http://127.0.0.1:9001/app/device-scan/intake")
    await page.getByTestId("device-scan-page").waitFor()
    for (const locale of ["zhCN", "en"]) {
      const response = await page.request.get(`http://127.0.0.1:9001/admin/devices/import-template?locale=${locale}`, { headers: { authorization: `Bearer ${token}` } })
      assert.equal(response.status(), 200, `Template response: ${response.status()} ${response.status() === 200 ? "" : await response.text()}`)
      await page.setContent(`<a href="/admin/devices/import-template?locale=${locale}" download="device-import-template.xlsx">Download</a>`)
      const [download] = await Promise.all([page.waitForEvent("download"), page.getByRole("link", { name: "Download" }).click()])
      assert.equal(await download.failure(), null)
      assert.equal(download.suggestedFilename(), "device-import-template.xlsx")
      const workbook = new ExcelJS.Workbook()
      await workbook.xlsx.readFile(await download.path())
      const source = workbook.getWorksheet("SKU source")!
      assert(source.rowCount > 1, "Live SKU source must contain records")
      assert.deepEqual(workbook.worksheets.filter((sheet) => sheet.state === "visible").map((sheet) => sheet.name), ["Devices", "SKU source"])
      assert.equal(workbook.getWorksheet("SKU search"), undefined)
      assert(!/\s/.test(source.getCell("F2").text), "SKU parameters must not require spaces")
      assert.equal(workbook.worksheets[0].getCell("A1").text, locale === "en" ? "Serial Number" : "序列号")
      const devices = workbook.worksheets[0]
      assert.equal(devices.getCell("E1").text, locale === "en" ? "SKU Parameters" : "SKU 参数")
      assert.equal(devices.getCell("F1").text, "SKU")
      assert((devices.model as any).sheetProtection.sheet)
      assert.equal(devices.getCell("E2").protection.locked, false)
      assert.notEqual(devices.getCell("F2").protection?.locked, false)
      assert(devices.getCell("F2").formula.includes("MATCH("))
      for (const address of ["E2", "E201"]) {
        assert(workbook.worksheets[0].getCell(address).dataValidation.formulae[0].includes("SKU choices"))
      }
      assert(workbook.worksheets[0].getCell("E2").dataValidation.prompt?.includes(locale === "en" ? "Search model" : "关键词"))
      devices.getCell("A2").value = "TEMPLATECHECK"
      devices.getCell("E2").value = source.getCell("F2").text
      const buffer = await workbook.xlsx.writeBuffer()
      const rows = await parseDeviceFile({ name: "devices.xlsx", arrayBuffer: async () => buffer } as unknown as File, (key) => key)
      assert.equal(rows.length, 1)
      assert.equal(rows[0].sku, source.getCell("A2").text)
      console.log(`PASS ${locale}: downloaded XLSX with ${source.rowCount - 1} SKU choices and validation on all 200 device rows`)
    }
  } finally {
    await browser.close()
  }
}
