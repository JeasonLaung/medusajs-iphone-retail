import { Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import { guardSupplierAdmin } from "../api/device-access"

function setup(path: string) {
  const user = { id: "supplier", email: "supplier@example.invalid", first_name: "Supplier", metadata: { device_role: "supplier", device_source_id: "source", device_order_ids: ["own-order"] } }
  const services = {
    [Modules.USER]: { listUsers: jest.fn().mockResolvedValue([user]), retrieveUser: jest.fn().mockResolvedValue(user) },
    [DEVICES_MODULE]: { listDeviceSources: jest.fn().mockResolvedValue([{ id: "source", type: "supplier" }]) },
  }
  const req: any = { auth_context: { actor_id: user.id, actor_type: "user" }, method: "GET", originalUrl: path, scope: { resolve: (key) => services[key] } }
  return { req, res: { json: jest.fn() }, next: jest.fn() }
}
test("supplier bootstrap returns only its own user without requiring broad native user permission", async () => {
  const { req, res, next } = setup("/admin/users/me")
  await guardSupplierAdmin(req, res as any, next)
  expect(res.json).toHaveBeenCalledWith({ user: expect.objectContaining({ id: "supplier" }) })
  expect(next).not.toHaveBeenCalled()
})
test("supplier bootstrap reports no native permissions and still forbids native orders", async () => {
  const { req, res, next } = setup("/admin/rbac/me/permissions")
  await guardSupplierAdmin(req, res as any, next)
  expect(res.json).toHaveBeenCalledWith({ permissions: [] })
  req.originalUrl = "/admin/orders"
  await expect(guardSupplierAdmin(req, res as any, next)).rejects.toThrow("Supplier access denied")
})
