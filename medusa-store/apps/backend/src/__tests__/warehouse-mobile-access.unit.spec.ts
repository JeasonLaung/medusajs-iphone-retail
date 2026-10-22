import { hasPermission } from "@medusajs/framework"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import { createIntakeToken } from "../lib/mobile-intake-token"
import { mobileIntakeAccess } from "../api/mobile-intake/access"
import { WAREHOUSE_ROLE } from "../lib/warehouse-access"

jest.mock("@medusajs/framework", () => ({ hasPermission: jest.fn().mockResolvedValue(true) }))
const secret = "warehouse-mobile-unit-secret"
function setup() {
  const user = { id: "actor", metadata: { device_role: "warehouse", warehouse_scope: "bound", warehouse_location_ids: ["own"] } }
  const services: any = {
    [Modules.USER]: { listUsers: jest.fn().mockImplementation(async () => [user]) },
    [ContainerRegistrationKeys.CONFIG_MODULE]: { projectConfig: { http: { jwtSecret: secret } } },
    [ContainerRegistrationKeys.QUERY]: { graph: jest.fn().mockResolvedValue({ data: [{ id: user.id, rbac_roles: [{ id: WAREHOUSE_ROLE }] }] }) },
    [DEVICES_MODULE]: { retrieveDeviceSource: jest.fn().mockResolvedValue({ id: "source", type: "self", location_id: "own" }) },
  }
  return { user, container: { resolve: (key) => services[key] } as any }
}
afterEach(() => (hasPermission as jest.Mock).mockReset().mockResolvedValue(true))

test("existing signed mobile links stop working after their warehouse binding is removed", async () => {
  const { user, container } = setup()
  const { token } = createIntakeToken("actor", "source", secret)
  await expect(mobileIntakeAccess(container, token)).resolves.toMatchObject({ source: { location_id: "own" } })
  user.metadata.warehouse_location_ids = []
  await expect(mobileIntakeAccess(container, token)).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
})
test("mobile sessions recheck role permissions on every use", async () => {
  const { container } = setup()
  const { token } = createIntakeToken("actor", "source", secret)
  await mobileIntakeAccess(container, token)
  ;(hasPermission as jest.Mock).mockResolvedValue(false)
  await expect(mobileIntakeAccess(container, token)).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
})
test("mobile return sessions check the signed return category", async () => {
  const { container } = setup()
  ;(hasPermission as jest.Mock).mockImplementation(async ({ actions }) => actions.resource === "warehouse_return")
  const returned = createIntakeToken("actor", "source", secret, Date.now(), "returned")
  const sale = createIntakeToken("actor", "source", secret)
  await expect(mobileIntakeAccess(container, returned.token)).resolves.toBeDefined()
  await expect(mobileIntakeAccess(container, sale.token)).rejects.toThrow("WAREHOUSE_ACCESS_DENIED")
})
