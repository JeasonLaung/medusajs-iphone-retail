import assert from "node:assert/strict"
import { dirname, resolve } from "node:path"
import type {
  ConfigModule,
  ExecArgs,
  IUserModuleService,
} from "@medusajs/framework/types"
import {
  ContainerRegistrationKeys,
  Modules,
  generateJwtToken,
} from "@medusajs/framework/utils"
import { localizeDeviceMessage } from "../admin/lib/device-messages"
import en from "../admin/i18n/json/en.json"
import zhCN from "../admin/i18n/json/zh-CN.json"

export default async function checkDeviceUi({ container }: ExecArgs) {
  const errors = en.deviceScan.errors
  assert.equal(
    localizeDeviceMessage(
      zhCN.deviceScan.errors.scanMismatch,
      (key) =>
        errors[key.replace("deviceScan.errors.", "") as keyof typeof errors],
    ),
    errors.scanMismatch,
    "Business validation errors translate to English",
  )
  assert.equal(
    localizeDeviceMessage("Unexpected provider error", () => "translated"),
    "Unexpected provider error",
    "Unknown provider errors retain their details",
  )
  assert(process.env.PLAYWRIGHT_MODULE_PATH, "Set PLAYWRIGHT_MODULE_PATH")
  const config: ConfigModule = container.resolve(
    ContainerRegistrationKeys.CONFIG_MODULE,
  )
  const users: IUserModuleService = container.resolve(Modules.USER)
  const [user] = await users.listUsers({})
  const token = generateJwtToken(
    {
      actor_id: user.id,
      actor_type: "user",
      auth_identity_id: "device_ui_check",
      app_metadata: { user_id: user.id },
    },
    { secret: config.projectConfig.http.jwtSecret, expiresIn: "5m" },
  )
  const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
  const library = require(
    require.resolve("@zxing/library", {
      paths: [dirname(require.resolve("@zxing/browser"))],
    }),
  )
  const matrix = new library.QRCodeWriter().encode(
    "DEVICECHECK123",
    library.BarcodeFormat.QR_CODE,
    256,
    256,
    new Map(),
  )
  let squares = ""
  for (let y = 0; y < 256; y++)
    for (let x = 0; x < 256; x++)
      if (matrix.get(x, y)) squares += `M${x},${y}h1v1h-1z`
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256"><rect width="256" height="256" fill="white"/><path d="${squares}" fill="black"/></svg>`
  const browser = await chromium.launch({
    channel: "chrome",
    headless: true,
    args: [
      "--use-fake-ui-for-media-stream",
      "--use-fake-device-for-media-stream",
    ],
  })
  try {
    const context = await browser.newContext({
      ignoreHTTPSErrors: true,
      viewport: { width: 390, height: 844 },
    })
    await context.addCookies([
      { name: "lng", value: "zhCN", url: "https://127.0.0.1:9443" },
    ])
    const pngPage = await context.newPage()
    await pngPage.setContent(svg)
    const imagePath = resolve(
      process.cwd(),
      "../../../logs/device-scan-test-qr.png",
    )
    await pngPage.locator("svg").screenshot({ path: imagePath })
    await pngPage.close()
    const page = await context.newPage()
    const errors: string[] = []
    page.on("pageerror", (error: Error) => errors.push(error.message))
    page.on("requestfailed", (request: any) =>
      console.log(
        "UI request failed:",
        request.url(),
        request.failure()?.errorText,
      ),
    )
    page.on("console", (message: any) => {
      if (message.type() === "error") console.log("UI browser:", message.text())
    })
    await page.route("**/admin/**", async (route: any) => {
      if (["fetch", "xhr"].includes(route.request().resourceType()))
        await route.continue({
          headers: {
            ...route.request().headers(),
            authorization: `Bearer ${token}`,
          },
        })
      else await route.continue()
    })
    for (let attempt = 0; attempt < 30; attempt++) {
      const health = await context.request.get("https://127.0.0.1:9443/health")
      if (health.ok()) break
      await new Promise((resolve) => setTimeout(resolve, 1000))
    }
    const response = await page.goto("https://127.0.0.1:9443/app/device-scan")
    assert(response?.ok(), "HTTPS upstream page is available")
    await page.getByTestId("device-scan-page").waitFor()
    await page.getByText("设备列表", { exact: false }).waitFor()
    assert(
      await page.evaluate(() => window.isSecureContext),
      "HTTPS secure context",
    )
    await page.getByLabel("上传标签图片").setInputFiles(imagePath)
    await page.getByText("已读取：DEVICECHECK123", { exact: true }).waitFor()
    assert.equal(
      await page.getByLabel("搜索设备").inputValue(),
      "DEVICECHECK123",
      "Decoded QR fills search",
    )
    await page.getByLabel("手动输入序列号、IMEI 或 EID").fill("123456789012345")
    await page.getByRole("button", { name: "使用号码", exact: true }).click()
    await page.getByText("已读取：123456789012345", { exact: true }).waitFor()
    await page
      .getByRole("button", { name: "打开手机相机", exact: true })
      .click()
    await page.waitForFunction(() => {
      const video = document.querySelector("video")
      return (
        video?.srcObject instanceof MediaStream &&
        video.srcObject.getVideoTracks().some((t) => t.readyState === "live")
      )
    })
    await page.getByRole("button", { name: "关闭相机", exact: true }).click()
    assert(
      await page.evaluate(() => {
        const video = document.querySelector("video")
        return (
          !video?.srcObject ||
          !(video.srcObject instanceof MediaStream) ||
          video.srcObject.getTracks().every((t) => t.readyState === "ended")
        )
      }),
      "Stop releases camera",
    )
    await page.goto("https://127.0.0.1:9443/app/device-scan/intake")
    await page.getByRole("heading", { name: "设备入库", exact: true }).waitFor()
    await page.getByLabel("上传标签图片").setInputFiles(imagePath)
    await page.waitForFunction(
      () =>
        (
          document.querySelector(
            'input[aria-label="苹果序列号"]',
          ) as HTMLInputElement
        )?.value === "DEVICECHECK123",
    )
    assert.equal(
      await page.getByLabel("苹果序列号", { exact: true }).inputValue(),
      "DEVICECHECK123",
      "Decoded QR fills intake identity",
    )
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
      "Mobile intake has no overflow",
    )
    await page.screenshot({
      path: resolve(
        process.cwd(),
        "../../../logs/device-scan-intake-mobile.png",
      ),
      fullPage: true,
    })
    await page.setViewportSize({ width: 1440, height: 1000 })
    await page.screenshot({
      path: resolve(
        process.cwd(),
        "../../../logs/device-scan-intake-desktop.png",
      ),
      fullPage: true,
    })
    await page
      .getByRole("link", { name: "仓库与供应商", exact: true })
      .click()
    await page.getByText("启用 SKU 设备库存", { exact: true }).waitFor()
    for (const locale of ["en", "zhCN"]) {
      await context.addCookies([
        { name: "lng", value: locale, url: "https://127.0.0.1:9443" },
      ])
      await page.reload()
      const english = locale === "en"
      const title = english ? "Device intake & dispatch" : "设备入库出库"
      await page.getByRole("link", { name: title, exact: true }).waitFor()
      const modes = english
        ? [
            "Find devices",
            "Device intake",
            "Allocate & dispatch",
            "Returns & inspection",
            "Warehouses & suppliers",
          ]
        : [
            "查设备",
            "设备入库",
            "配货出库",
            "退回检测",
            "仓库与供应商",
          ]
      for (const [index, name] of modes.entries()) {
        await page.goto(`https://127.0.0.1:9443/app/device-scan/${["lookup", "intake", "orders", "inspection", "sources"][index]}`)
        await page.getByRole("heading", { name, exact: true }).waitFor()
        if (name === modes[1]) {
          await page
            .getByLabel(english ? "Import device XLSX" : "导入设备 XLSX")
            .setInputFiles({
              name: "invalid.csv",
              mimeType: "text/csv",
              buffer: Buffer.from('imei,notes\n123,"unclosed'),
            })
          await page
            .locator("[data-sonner-toast]")
            .filter({
              hasText: english ? "Choose an XLSX file." : "请选择 XLSX 文件",
            })
            .waitFor()
        }
      }
      await page
        .getByText(
          english ? "Enable device inventory for SKU" : "启用 SKU 设备库存",
          { exact: true },
        )
        .waitFor()
      await page.goto("https://127.0.0.1:9443/app/device-scan/lookup")
      await page
        .getByLabel(
          english
            ? "Enter serial number, IMEI or EID manually"
            : "手动输入序列号、IMEI 或 EID",
        )
        .fill("DEVICECHECK123")
      await page
        .getByRole("button", {
          name: english ? "Use identifier" : "使用号码",
          exact: true,
        })
        .click()
      assert.equal(
        await page
          .getByLabel(english ? "Search devices" : "搜索设备")
          .inputValue(),
        "DEVICECHECK123",
      )
      await page.setViewportSize({ width: 390, height: 844 })
      assert(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
        `${locale} mobile page has no overflow`,
      )
      await page.screenshot({
        path: resolve(
          process.cwd(),
          `../../../logs/device-scan-${locale}-mobile.png`,
        ),
        fullPage: true,
      })
      await page.setViewportSize({ width: 1440, height: 1000 })
    }
    assert.equal(errors.length, 0, errors.join("\n"))
    console.log(
      "Device UI checks passed: Chinese and English sidebar and all five modes, localized XLSX validation, HTTPS admin, QR image decoding, manual scanner entry, intake field routing, camera start/stop, responsive pages and no runtime errors.",
    )
  } finally {
    await browser.close()
  }
}
