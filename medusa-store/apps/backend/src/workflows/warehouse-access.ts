import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys, MedusaError, Modules } from "@medusajs/framework/utils"
import { assertWarehouseAdministrator } from "../lib/warehouse-role"
import { WAREHOUSE_ROLE } from "../lib/warehouse-access"

export type WarehouseBinding = { actor_id: string; user_id: string; scope: "platform" | "all" | "bound"; location_ids: string[] }
const updateWarehouseBinding = createStep("update-warehouse-binding", async (input: WarehouseBinding, { container }) => {
  await assertWarehouseAdministrator(container, input.actor_id)
  if (input.user_id === input.actor_id) throw new MedusaError(MedusaError.Types.INVALID_DATA, "WAREHOUSE_SELF_CHANGE_DENIED")
  const users = container.resolve(Modules.USER)
  const user = await users.retrieveUser(input.user_id)
  if (user.metadata?.device_role === "supplier") throw new MedusaError(MedusaError.Types.INVALID_DATA, "WAREHOUSE_SUPPLIER_ACCOUNT_DENIED")
  if (input.scope === "bound") {
    if (!input.location_ids.length) throw new MedusaError(MedusaError.Types.INVALID_DATA, "WAREHOUSE_LOCATION_REQUIRED")
    const locations = await container.resolve(Modules.STOCK_LOCATION).listStockLocations({ id: input.location_ids }, { take: null })
    if (locations.length !== input.location_ids.length) throw new MedusaError(MedusaError.Types.INVALID_DATA, "WAREHOUSE_LOCATION_INVALID")
  }
  const [warehouseRole] = await container.resolve(Modules.RBAC).listRbacRoles({ id: WAREHOUSE_ROLE })
  if (!warehouseRole) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, "WAREHOUSE_ROLE_NOT_INITIALIZED")
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data }: { data: any[] } = await query.graph({ entity: "user", fields: ["id", "rbac_roles.id"], filters: { id: user.id } })
  const originalRoles = data[0]?.rbac_roles?.filter(Boolean).map((role) => role!.id) || []
  const link = container.resolve(ContainerRegistrationKeys.LINK)
  const metadata = { ...user.metadata, device_role: input.scope === "platform" ? "admin" : "warehouse", warehouse_scope: input.scope === "platform" ? null : input.scope, warehouse_location_ids: input.scope === "bound" ? input.location_ids : null }
  const desired = input.scope === "platform" ? "role_super_admin" : WAREHOUSE_ROLE
  const opposite = input.scope === "platform" ? WAREHOUSE_ROLE : "role_super_admin"
  // Persist the restriction first; a role update failure must not leave widened access.
  await users.updateUsers({ id: user.id, metadata })
  try {
    if (!originalRoles.includes(desired)) await link.create({ [Modules.USER]: { user_id: user.id }, [Modules.RBAC]: { rbac_role_id: desired } })
    if (originalRoles.includes(opposite)) await link.dismiss({ [Modules.USER]: { user_id: user.id }, [Modules.RBAC]: { rbac_role_id: opposite } })
  } catch (error) {
    if (!originalRoles.includes(desired)) await link.dismiss({ [Modules.USER]: { user_id: user.id }, [Modules.RBAC]: { rbac_role_id: desired } })
    if (originalRoles.includes(opposite)) await link.create({ [Modules.USER]: { user_id: user.id }, [Modules.RBAC]: { rbac_role_id: opposite } })
    await users.updateUsers({ id: user.id, metadata: user.metadata })
    throw error
  }
  return new StepResponse({ user_id: user.id, scope: input.scope, location_ids: input.scope === "bound" ? input.location_ids : [] })
})

export const updateWarehouseAccessWorkflow = createWorkflow("update-warehouse-access", function (input: WarehouseBinding) { return new WorkflowResponse(updateWarehouseBinding(input)) })
