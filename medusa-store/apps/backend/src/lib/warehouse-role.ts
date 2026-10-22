import type { MedusaContainer } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { WAREHOUSE_ROLE, warehouseDenied } from "./warehouse-access"

export async function assertWarehouseAdministrator(container: MedusaContainer, actorId: string) {
  const { data }: { data: any[] } = await container.resolve(ContainerRegistrationKeys.QUERY).graph({ entity: "user", fields: ["id", "metadata", "rbac_roles.id"], filters: { id: actorId } })
  if (!data[0] || data[0].rbac_roles?.some((role) => role?.id === WAREHOUSE_ROLE) || data[0].metadata?.device_role === "warehouse" || Array.isArray(data[0].metadata?.warehouse_location_ids) || !data[0].rbac_roles?.some((role) => role?.id === "role_super_admin")) throw warehouseDenied()
}

export async function ensureWarehouseRole(container: MedusaContainer) {
  const rbac = container.resolve(Modules.RBAC)
  let [role] = await rbac.listRbacRoles({ id: WAREHOUSE_ROLE })
  if (!role) {
    const input = { id: WAREHOUSE_ROLE, name: "Warehouse operator", description: "Orders, device intake and returns within assigned warehouses. Products and SKUs are shared." }
    role = await rbac.createRbacRoles(input)
  }
  const grants = [
    ["order", "read"], ["product", "read"], ["product_variant", "read"], ["product_type", "read"], ["product_tag", "read"], ["product_category", "read"], ["product_collection", "read"],
    ["region", "read"], ["sales_channel", "read"], ["store", "read"], ["user", "read"], ["stock_location", "read"], ["return_reason", "read"],
    ["warehouse_intake", "read"], ["warehouse_intake", "update"], ["warehouse_return", "read"], ["warehouse_return", "update"], ["warehouse_allocation", "update"],
  ]
  for (const [resource, operation] of grants) {
    const id = `rpol_retail_${resource}_${operation}`
    let [existing] = await rbac.listRbacPolicies({ key: `${resource}:${operation}` })
    if (!existing) {
      const input = { id, key: `${resource}:${operation}`, resource, operation, name: `${resource}:${operation}` }
      existing = await rbac.createRbacPolicies(input)
    }
    const linkId = `rlpl_retail_${resource}_${operation}`
    const [linked] = await rbac.listRbacRolePolicies({ id: linkId })
    if (!linked) {
      const input = { id: linkId, role_id: role.id, policy_id: existing.id }
      await rbac.createRbacRolePolicies(input)
    }
  }
  return role
}
