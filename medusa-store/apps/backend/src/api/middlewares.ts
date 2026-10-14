import {
  authenticate,
  configureStoreSearch,
  defineMiddlewares,
  validateAndTransformBody,
} from "@medusajs/framework/http"
import { CreateAfterSales } from "./store/orders/[id]/after-sales/validators"
import { UpdateAfterSales } from "./admin/orders/[id]/after-sales/validators"
import { DeviceCommand, DeviceRefund } from "./admin/devices/validators"
import {
  guardDeviceOrder,
  guardDeviceInventory,
  guardDeviceReturn,
} from "./device-guards"
import { guardSupplierAdmin } from "./device-access"
import { SupplierAccountBody } from "./admin/devices/supplier-accounts/route"

// The product index declares filterable `status` and `sales_channel_ids`, so
// the route narrows it to published products in the key's sales channels.
export default defineMiddlewares({
  routes: [
    {
      matcher: "/admin/*",
      middlewares: [
        authenticate("user", ["session", "bearer", "api-key"], {
          allowUnauthenticated: true,
          allowUnregistered: true,
          requireMfa: true,
        }),
        guardSupplierAdmin,
      ],
    },
    {
      matcher: "/admin/devices/supplier-accounts",
      method: ["POST"],
      middlewares: [validateAndTransformBody(SupplierAccountBody)],
    },
    {
      matcher: "/admin/devices/refunds",
      method: ["POST"],
      middlewares: [validateAndTransformBody(DeviceRefund)],
    },
    ...[
      "/admin/orders/:id/fulfillments",
      "/admin/orders/:id/fulfillments/:fulfillment_id/shipments",
      "/admin/orders/:id/fulfillments/:fulfillment_id/cancel",
      "/admin/orders/:id/cancel",
      "/admin/orders/:id/edits",
    ].map((matcher) => ({
      matcher,
      method: "POST" as const,
      middlewares: [guardDeviceOrder],
    })),
    ...[
      "/admin/inventory-items/:id/location-levels",
      "/admin/inventory-items/:id/location-levels/:location_id",
      "/admin/inventory-items/:id/location-levels/batch",
    ].map((matcher) => ({
      matcher,
      method: ["POST", "DELETE"] as ("POST" | "DELETE")[],
      middlewares: [guardDeviceInventory],
    })),
    ...[
      "/admin/returns",
      "/admin/returns/:id/receive/confirm",
      "/store/returns",
      "/admin/exchanges",
      "/admin/exchanges/:id/request/confirm",
    ].map((matcher) => ({
      matcher,
      method: "POST" as const,
      middlewares: [guardDeviceReturn],
    })),
    {
      matcher: "/admin/inventory-items/:id",
      method: ["DELETE"],
      middlewares: [guardDeviceInventory],
    },
    {
      matcher: "/admin/devices",
      method: ["POST"],
      middlewares: [validateAndTransformBody(DeviceCommand)],
    },
    {
      matcher: "/store/orders/:id/devices",
      method: ["GET"],
      middlewares: [authenticate("customer", ["session", "bearer"])],
    },
    {
      matcher: "/admin/orders/:id/after-sales",
      method: ["POST"],
      middlewares: [validateAndTransformBody(UpdateAfterSales)],
    },
    {
      matcher: "/store/orders/:id/after-sales",
      method: ["GET", "POST"],
      middlewares: [authenticate("customer", ["session", "bearer"])],
    },
    {
      matcher: "/store/orders/:id/after-sales",
      method: ["POST"],
      middlewares: [validateAndTransformBody(CreateAfterSales)],
    },
    {
      method: ["POST"],
      matcher: "/store/search",
      middlewares: [
        configureStoreSearch({
          allowed_indexes: {
            product: true,
          },
        }),
      ],
    },
  ],
})
