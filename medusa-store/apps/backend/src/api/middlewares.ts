import {
  authenticate,
  configureStoreSearch,
  defineMiddlewares,
  validateAndTransformBody,
  validateAndTransformQuery,
} from "@medusajs/framework/http"
import { CreateAfterSales } from "./store/orders/[id]/after-sales/validators"
import { UpdateAfterSales } from "./admin/orders/[id]/after-sales/validators"
import { DeviceCommand, DeviceRefund } from "./admin/devices/validators"
import { CreateMobileIntake, MobileIntakeBody } from "./mobile-intake/validators"
import { DevicePhotoUpload } from "./device-photo-validators"
import {
  guardDeviceOrder,
  guardDeviceInventory,
  guardDeviceReturn,
} from "./device-guards"
import { WarehouseBindingBody } from "./admin/warehouse-access/validators"
import { guardWarehouseAdmin } from "./warehouse-guards"
import { guardSupplierAdmin } from "./device-access"
import { SupplierAccountBody } from "./admin/devices/supplier-accounts/route"
import { MailCommand, MailListQuery } from "./admin/mail/validators"
import { CreateReview, ModerateReview, ReviewListQuery, ReviewPurchasesQuery } from "./review-validators"
import {
  guardManualPaymentSelection,
  guardMarkAsPaid,
  guardManualCartCompletion,
  guardManualPaymentAuthorization,
  guardManualPaymentCapture,
  listProductionPaymentProviders,
} from "./payment-guards"

// The product index declares filterable `status` and `sales_channel_ids`, so
// the route narrows it to published products in the key's sales channels.
export default defineMiddlewares({
  routes: [
    { matcher: "/admin/warehouse-access", method: "POST", middlewares: [validateAndTransformBody(WarehouseBindingBody)] },
    ...["/admin/devices/photos", "/mobile-intake/photos"].map((matcher) => ({ matcher, method: "POST" as const, bodyParser: { sizeLimit: "34mb" }, middlewares: [validateAndTransformBody(DevicePhotoUpload)] })),
    { matcher: "/admin/devices/mobile-intake", method: "POST", middlewares: [validateAndTransformBody(CreateMobileIntake)] },
    { matcher: "/mobile-intake", method: "POST", bodyParser: { sizeLimit: "32kb" }, middlewares: [validateAndTransformBody(MobileIntakeBody)] },
    { matcher: "/store/product-reviews/purchases", method: "GET", middlewares: [authenticate("customer", ["session", "bearer"]), validateAndTransformQuery(ReviewPurchasesQuery, {})] },
    { matcher: "/store/orders/:id/product-reviews", method: "GET", middlewares: [authenticate("customer", ["session", "bearer"])] },
    { matcher: "/store/product-reviews", method: "GET", middlewares: [validateAndTransformQuery(ReviewListQuery, {})] },
    { matcher: "/store/product-reviews", method: "POST", middlewares: [authenticate("customer", ["session", "bearer"]), validateAndTransformBody(CreateReview)] },
    { matcher: "/admin/product-reviews", method: "GET", middlewares: [validateAndTransformQuery(ReviewListQuery, {})] },
    { matcher: "/admin/product-reviews/:id", method: "POST", middlewares: [validateAndTransformBody(ModerateReview)] },
    { matcher: "/admin/mail", method: "GET", middlewares: [validateAndTransformQuery(MailListQuery, {})] },
    { matcher: "/admin/mail/commands", method: "POST", bodyParser: { sizeLimit: "16mb" }, middlewares: [validateAndTransformBody(MailCommand)] },
    { matcher: "/mail/webhooks/:account_id", method: "POST", bodyParser: { preserveRawBody: true, sizeLimit: "1mb" }, middlewares: [] },
    {
      matcher: "/admin/*",
      middlewares: [
        authenticate("user", ["session", "bearer", "api-key"], {
          allowUnauthenticated: true,
          allowUnregistered: true,
          requireMfa: true,
        }),
        guardSupplierAdmin,
        guardWarehouseAdmin,
      ],
    },
    ...["/store/payment-providers", "/admin/payments/payment-providers"].map((matcher) => ({
      matcher,
      method: "GET" as const,
      middlewares: [listProductionPaymentProviders],
    })),
    ...["/store/payment-collections/:id/payment-sessions", "/admin/payment-collections/:id/payment-sessions"].map((matcher) => ({
      matcher,
      method: "POST" as const,
      middlewares: [guardManualPaymentSelection],
    })),
    {
      matcher: "/store/carts/:id/complete",
      method: "POST",
      middlewares: [guardManualCartCompletion],
    },
    {
      matcher: "/admin/payment-collections/:id/mark-as-paid",
      method: "POST",
      middlewares: [guardMarkAsPaid],
    },
    {
      matcher: "/admin/orders/:id/payment-sessions/authorize",
      method: "POST",
      middlewares: [guardManualPaymentAuthorization],
    },
    {
      matcher: "/admin/payments/:id/capture",
      method: "POST",
      middlewares: [guardManualPaymentCapture],
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
