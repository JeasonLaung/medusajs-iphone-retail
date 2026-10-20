import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { deviceAccess, supplierDenied } from "../../../device-access"
import { createDeviceTemplate, type TemplateSku } from "../../../../lib/device-import-template"
import en from "../../../../admin/i18n/json/en.json"
import zhCN from "../../../../admin/i18n/json/zh-CN.json"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const access = await deviceAccess(req.scope, req.auth_context.actor_id)
  if (access.role !== "admin") throw supplierDenied()
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const variants: TemplateSku[] = []
  for (let skip = 0; ; skip += 1000) {
    const { data } = await query.graph({
      entity: "product_variant",
      fields: ["sku", "title", "product.title", "options.value", "options.option.title"],
      pagination: { take: 1000, skip, order: { sku: "ASC", id: "ASC" } },
    })
    variants.push(...data)
    if (data.length < 1000) break
  }
  const messages = String(req.query.locale || "").toLowerCase().startsWith("zh") ? zhCN.deviceScan : en.deviceScan
  const buffer = await createDeviceTemplate(variants, { skuPrompt: messages.templateSkuPrompt, skuError: messages.templateSkuError }, String(req.query.locale || "en"))
  res.setHeader("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
  res.setHeader("Content-Disposition", 'attachment; filename="device-import-template.xlsx"')
  res.setHeader("Cache-Control", "no-store")
  res.send(Buffer.from(buffer))
}
