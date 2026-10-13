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
import { parseDeviceCsv } from "../admin/lib/device-csv"

export default async function checkDeviceUi({ container }: ExecArgs) {
  assert.equal(
    parseDeviceCsv(
      '\uFEFFserial_number,notes\r\nDEVICECHECK123,"quoted, note"\r\n',
    )[0].notes,
    "quoted, note",
  )
  assert.equal(
    parseDeviceCsv('imei,notes\n123456789012345,"two\nlines and ""quotes"""')[0]
      .notes,
    'two\nlines and "quotes"',
  )
  assert.throws(() => parseDeviceCsv('imei,notes\n123,"unterminated'))
  assert.throws(() => parseDeviceCsv("imei,imei\n123,123"))
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
    await page.getByLabel("手动输入序列号或 IMEI").fill("123456789012345")
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
    await page
      .getByRole("button", { name: "扫码入库 / 导入", exact: true })
      .click()
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
      .getByRole("button", { name: "仓库 / 供应商库存", exact: true })
      .click()
    await page.getByText("启用 SKU 设备库存", { exact: true }).waitFor()
    assert.equal(errors.length, 0, errors.join("\n"))
    console.log(
      "Device UI checks passed: CSV quoting/BOM/errors, HTTPS admin, QR image decoding, manual scanner entry, intake field routing, camera start/stop with a simulated video device, responsive pages and no runtime errors.",
    )
  } finally {
    await browser.close()
  }
}
