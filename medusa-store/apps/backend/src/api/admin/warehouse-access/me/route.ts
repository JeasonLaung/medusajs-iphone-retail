import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { resolvePermissions } from "@medusajs/framework"
import { warehouseAccess } from "../../../../lib/warehouse-access"
export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const access = await warehouseAccess(req.scope, req.auth_context.actor_id)
  if (!access.warehouse) return res.json({ ...access, permissions: [] })
  const { data }: { data: any[] } = await req.scope.resolve(ContainerRegistrationKeys.QUERY).graph({ entity: "user", fields: ["id", "rbac_roles.id"], filters: { id: req.auth_context.actor_id } })
  const permissions = await resolvePermissions({ container: req.scope, roles: data[0]?.rbac_roles?.filter(Boolean).map((role) => role.id) || [], universe: ["order", "warehouse_intake", "warehouse_return", "warehouse_allocation"].flatMap((resource) => ["read", "update"].map((operation) => ({ resource, operation }))) })
  res.json({ ...access, permissions: [...permissions] })
}
