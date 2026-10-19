import assert from "node:assert/strict"
import type { ConfigModule, ExecArgs, IUserModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules, generateJwtToken } from "@medusajs/framework/utils"
import ExcelJS from "exceljs"

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
      for (const address of ["D2", "D201"]) {
        assert(workbook.worksheets[0].getCell(address).dataValidation.formulae[0].includes("SKU choices"))
      }
      assert(workbook.worksheets[0].getCell("D2").dataValidation.prompt?.includes(locale === "en" ? "keywords" : "关键词"))
      console.log(`PASS ${locale}: downloaded XLSX with ${source.rowCount - 1} SKU choices and validation on all 200 device rows`)
    }
  } finally {
    await browser.close()
  }
}
