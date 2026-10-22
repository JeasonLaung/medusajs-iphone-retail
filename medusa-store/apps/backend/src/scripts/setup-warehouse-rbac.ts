import type { ExecArgs } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, MedusaError, Modules } from "@medusajs/framework/utils"
import { ensureWarehouseRole } from "../lib/warehouse-role"

// Explicit one-time migration of existing unrestricted accounts. New users are
// never promoted automatically by startup, login, or permission checks.
export default async function setupWarehouseRbac({ container }: ExecArgs) {
  const rbac = container.resolve(Modules.RBAC)
  const role = await ensureWarehouseRole(container)
  const [admin] = await rbac.listRbacRoles({ id: "role_super_admin" })
  if (!admin) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, "Native Super Admin role was not initialized")
  if (admin.metadata?.retail_bootstrap_complete) return console.log("RBAC already initialized; existing user permissions preserved")
  const users = await container.resolve(Modules.USER).listUsers({}, { take: null })
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const link = container.resolve(ContainerRegistrationKeys.LINK)
  for (const user of users) {
    if (user.metadata?.device_role === "supplier") continue
    const { data }: { data: any[] } = await query.graph({ entity: "user", fields: ["id", "rbac_roles.id"], filters: { id: user.id } })
    const target = user.metadata?.device_role === "warehouse" ? role.id : admin.id
    if (!data[0]?.rbac_roles?.some((assigned) => assigned?.id === target)) await link.create({ [Modules.USER]: { user_id: user.id }, [Modules.RBAC]: { rbac_role_id: target } })
  }
  await rbac.updateRbacRoles({ id: admin.id, metadata: { ...admin.metadata, retail_bootstrap_complete: true } })
  console.log("RBAC initialized; existing platform administrators retain full access")
}
