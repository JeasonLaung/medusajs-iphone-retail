import type {
  MedusaContainer,
  IUserModuleService,
} from "@medusajs/framework/types"
import type {
  AuthenticatedMedusaRequest,
  MedusaResponse,
  MedusaNextFunction,
} from "@medusajs/framework/http"
import { MedusaError, Modules } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"
import type { DeviceCommandBody } from "./admin/devices/validators"

export const supplierDenied = () =>
  new MedusaError(MedusaError.Types.FORBIDDEN, "Supplier access denied")

export async function deviceAccess(
  container: MedusaContainer,
  actorId: string,
) {
  const users: IUserModuleService = container.resolve(Modules.USER)
  const [user] = await users.listUsers({ id: actorId })
  if (!user) {
    const [key] = await container
      .resolve(Modules.API_KEY)
      .listApiKeys({ id: actorId, type: "secret" })
    if (key)
      return { role: "admin", source_id: null, order_ids: [] as string[] }
    throw supplierDenied()
  }
  const supplier = user.metadata?.device_role === "supplier"
  const sourceId =
    supplier && typeof user.metadata?.device_source_id === "string"
      ? user.metadata.device_source_id
      : null
  const orderIds =
    supplier && Array.isArray(user.metadata?.device_order_ids)
      ? user.metadata.device_order_ids.filter(
          (id): id is string => typeof id === "string",
        )
      : []
  if (supplier) {
    if (!sourceId) throw supplierDenied()
    const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
    const [source] = await service.listDeviceSources({
      id: sourceId,
      type: "supplier",
    })
    if (!source) throw supplierDenied()
  }
  return {
    role: supplier ? "supplier" : "admin",
    source_id: sourceId,
    order_ids: orderIds,
  }
}

// Suppliers use the same authenticated dashboard, but cannot call its unrestricted APIs.
export async function guardSupplierAdmin(
  req: AuthenticatedMedusaRequest,
  res: MedusaResponse,
  next: MedusaNextFunction,
) {
  // Public flags and invite acceptance retain their native authentication rules.
  if (!req.auth_context?.actor_id || req.auth_context.actor_type === "api-key")
    return next()
  const access = await deviceAccess(req.scope, req.auth_context.actor_id)
  if (access.role !== "supplier") return next()
  const path = req.originalUrl.split("?")[0].replace(/\/$/, "")
  if (
    path === `/admin/users/${req.auth_context.actor_id}` &&
    req.method === "POST" &&
    req.body &&
    Object.keys(req.body).every((key) =>
      ["first_name", "last_name"].includes(key),
    )
  )
    return next()
  if (path === "/admin/devices" && ["GET", "POST"].includes(req.method))
    return next()
  // These bootstrap responses must remain usable by scoped supplier accounts
  // that intentionally hold no native order/customer permissions.
  if (req.method === "GET" && path === "/admin/users/me") {
    const user = await req.scope.resolve<IUserModuleService>(Modules.USER).retrieveUser(req.auth_context.actor_id)
    return res.json({ user: { id: user.id, email: user.email, first_name: user.first_name, last_name: user.last_name, avatar_url: user.avatar_url, metadata: user.metadata } })
  }
  if (req.method === "GET" && path === "/admin/rbac/me/permissions") return res.json({ permissions: [] })
  if (req.method === "GET" && (/^\/admin\/layouts\//.test(path) || path === "/admin/plugins" || path === "/admin/notifications")) {
    if (path === "/admin/notifications") return res.json({ notifications: [], count: 0, offset: 0, limit: 1 })
    return next()
  }
  if (path === "/admin/warehouse-access/me" && req.method === "GET") return next()
  if (path === "/admin/devices/variants" && req.method === "GET") return next()
  if (
    req.method === "GET" &&
    ["/admin/users/me", "/admin/feature-flags"].includes(path)
  )
    return next()
  // The dashboard header only needs the store's name and ID.
  if (path === "/admin/stores" && req.method === "GET") {
    const stores = await req.scope
      .resolve(Modules.STORE)
      .listStores({}, { take: 1 })
    return res.json({
      stores: stores.map((store) => ({ id: store.id, name: store.name })),
      count: stores.length,
      offset: 0,
      limit: 1,
    })
  }
  throw supplierDenied()
}

export async function authorizeDeviceCommand(
  container: MedusaContainer,
  actorId: string,
  body: DeviceCommandBody,
) {
  const access = await deviceAccess(container, actorId)
  if (access.role !== "supplier") return
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const ownSource = (id: string) => {
    if (id !== access.source_id) throw supplierDenied()
  }
  const ownOrder = (id: string) => {
    if (!access.order_ids.includes(id)) throw supplierDenied()
  }
  const ownAssignment = (a: { source_id: string; order_id: string }) => {
    ownSource(a.source_id)
    ownOrder(a.order_id)
  }
  switch (body.action) {
    case "supply":
      ownSource(body.source_id)
      return
    case "supplier_assign":
      ownSource(body.device.source_id)
      ownOrder(body.order_id)
      if (body.replacement_of) throw supplierDenied()
      return
    case "assign": {
      const device = await service.retrieveDevice(body.device_id)
      const source = await service.retrieveDeviceSource(access.source_id!)
      ownSource(device.source_id)
      if (device.location_id !== source.location_id || body.replacement_of)
        throw supplierDenied()
      ownOrder(body.order_id)
      return
    }
    case "release":
      ownAssignment(await service.retrieveDeviceAssignment(body.assignment_id))
      return
    case "fulfill": {
      const assignments = await service.listDeviceAssignments({
        id: body.assignment_ids,
      })
      if (
        !assignments.length ||
        assignments.length !== body.assignment_ids.length
      )
        throw supplierDenied()
      assignments.forEach(ownAssignment)
      return
    }
    case "ship":
    case "cancel_fulfillment": {
      const assignments = await service.listDeviceAssignments({
        fulfillment_id: body.fulfillment_id,
      })
      if (!assignments.length) throw supplierDenied()
      assignments.forEach(ownAssignment)
      return
    }
    default:
      throw supplierDenied()
  }
}
