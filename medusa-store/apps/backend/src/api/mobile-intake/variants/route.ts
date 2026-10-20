import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"
import { mobileIntakeAccess } from "../access"
import { deviceVariantSearchFilters } from "../../../lib/device-variant-search"
import { requiresImei } from "../../../modules/devices/identity"

export async function GET(req: MedusaRequest, res: MedusaResponse) {
  await mobileIntakeAccess(req.scope, String(req.headers["x-intake-token"] || ""))
  const search = typeof req.query.q === "string" ? req.query.q.trim() : ""
  if (search.length > 200 || search.split(/\s+/).length > 12)
    throw new MedusaError(MedusaError.Types.INVALID_DATA, "MOBILE_INTAKE_SEARCH_INVALID")
  const offset = Math.max(0, Math.min(100000, Math.floor(Number(req.query.offset) || 0)))
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const { data, metadata } = await query.graph({
    entity: "product_variant",
    fields: ["id", "sku", "title", "product.title", "product.metadata", "product.type.value"],
    filters: deviceVariantSearchFilters(search),
    pagination: { take: 20, skip: offset, order: { sku: "ASC", id: "ASC" } },
  })
  res.setHeader("Cache-Control", "no-store")
  res.json({ variants: data.filter((v) => v.sku).map((v) => ({ id: v.id, sku: v.sku, title: `${v.product?.title || ""} · ${v.title}`, requires_imei: requiresImei(v.product) })), count: metadata?.count || 0, offset })
}
