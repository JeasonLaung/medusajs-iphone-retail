import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"
import { deviceVariantSearchFilters } from "../../../../lib/device-variant-search"
import { deviceAccess } from "../../../device-access"
import { requiresImei } from "../../../../modules/devices/identity"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  await deviceAccess(req.scope, req.auth_context.actor_id)
  const search = typeof req.query.q === "string" ? req.query.q : ""
  const sku = typeof req.query.sku === "string" ? req.query.sku.trim() : ""
  const id = typeof req.query.id === "string" ? req.query.id.trim() : ""
  if (search.length > 200 || search.trim().split(/\s+/).length > 12) {
    throw new MedusaError(MedusaError.Types.INVALID_DATA, "Search is too long")
  }
  const offset = Math.max(0, Math.min(100000, Math.floor(Number(req.query.offset) || 0)))
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const { data: variants, metadata } = await query.graph({
    entity: "product_variant",
    fields: ["id", "title", "sku", "product.title", "product.handle", "product.metadata", "product.type.value", "options.value", "inventory_items.inventory.location_levels.*"],
    filters: { ...deviceVariantSearchFilters(search), ...(sku ? { sku } : {}), ...(id ? { id } : {}) },
    pagination: { take: 20, skip: offset, order: { sku: "ASC", id: "ASC" } },
  })
  res.json({ variants: variants.map((variant) => ({ ...variant, requires_imei: requiresImei(variant.product) })), count: metadata?.count || 0, offset, limit: 20 })
}
