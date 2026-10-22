import { createStep } from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { WAREHOUSE_ROLE } from "../lib/warehouse-access"
import { assertWarehouseAdministrator } from "../lib/warehouse-role"
import "../workflows/warehouse-access"

jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn(() => ({})), createWorkflow: jest.fn(() => ({})),
  StepResponse: class { constructor(public output: unknown) {} }, WorkflowResponse: jest.fn(),
}))
const save = (createStep as jest.Mock).mock.calls.find(([name]) => name === "update-warehouse-binding")[1]
function setup() {
  const target = { id: "target", metadata: { device_role: "admin", keep: "original" } }
  const actor = { id: "actor", metadata: {}, rbac_roles: [{ id: "role_super_admin" }] }
  const users = { retrieveUser: jest.fn().mockResolvedValue(target), updateUsers: jest.fn().mockResolvedValue(undefined) }
  const link = { create: jest.fn().mockResolvedValue(undefined), dismiss: jest.fn().mockResolvedValue(undefined) }
  const services = {
    [Modules.USER]: users,
    [Modules.STOCK_LOCATION]: { listStockLocations: jest.fn().mockResolvedValue([{ id: "own" }]) },
    [Modules.RBAC]: { listRbacRoles: jest.fn().mockResolvedValue([{ id: WAREHOUSE_ROLE }]) },
    [ContainerRegistrationKeys.QUERY]: { graph: jest.fn().mockImplementation(async ({ filters }) => ({ data: [filters.id === "actor" ? actor : { ...target, rbac_roles: [{ id: "role_super_admin" }] }] })) },
    [ContainerRegistrationKeys.LINK]: link,
  }
  return { actor, target, users, link, container: { resolve: (key) => services[key] } as any }
}
const input = { actor_id: "actor", user_id: "target", scope: "bound", location_ids: ["own"] }

test("warehouse role holders cannot administer bindings even if a super-admin link remains", async () => {
  const { actor, container } = setup()
  actor.rbac_roles.push({ id: WAREHOUSE_ROLE })
  await expect(assertWarehouseAdministrator(container, "actor")).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
})
test("self changes are rejected before any user update", async () => {
  const { container, users, link } = setup()
  await expect(save({ ...input, user_id: "actor" }, { container })).rejects.toThrow("WAREHOUSE_SELF_CHANGE_DENIED")
  expect(users.updateUsers).not.toHaveBeenCalled()
  expect(link.create).not.toHaveBeenCalled()
})
test("bindings require real warehouses and preserve unrelated user metadata", async () => {
  const { container, users } = setup()
  await expect(save({ ...input, location_ids: [] }, { container })).rejects.toThrow("WAREHOUSE_LOCATION_REQUIRED")
  expect(users.updateUsers).not.toHaveBeenCalled()
  await save(input, { container })
  expect(users.updateUsers).toHaveBeenCalledWith({ id: "target", metadata: { keep: "original", device_role: "warehouse", warehouse_scope: "bound", warehouse_location_ids: ["own"] } })
})
test("a failed role removal restores metadata without duplicating a still-existing role", async () => {
  const { container, users, link, target } = setup()
  link.dismiss.mockRejectedValueOnce(new Error("link failure"))
  await expect(save(input, { container })).rejects.toThrow("link failure")
  expect(link.create).toHaveBeenCalledTimes(1)
  expect(link.dismiss).toHaveBeenLastCalledWith({ [Modules.USER]: { user_id: "target" }, [Modules.RBAC]: { rbac_role_id: WAREHOUSE_ROLE } })
  expect(users.updateUsers).toHaveBeenLastCalledWith({ id: "target", metadata: target.metadata })
})
