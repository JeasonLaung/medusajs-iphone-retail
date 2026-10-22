import path from "node:path"
import fs from "node:fs"
import { MedusaError } from "@medusajs/framework/utils"
import { transformOrderCancellation } from "./admin-order-cancellation"
import type { DepOptimizationOptions, Plugin } from "vite" with { "resolution-mode": "import" }

// Extend the installed dashboard at its form/row boundaries, without vendor edits.
// These anchors are checked so dashboard upgrades cannot silently drop allocation.
export function transformDeviceFulfillment(source: string, id: string) {
  const filename = id.replace(/\\/g, "/").split("?")[0]
  const form = /\/@medusajs\/dashboard\/dist\/order-create-fulfillment-[^/]+\.mjs$/.test(filename)
  const detail = /\/@medusajs\/dashboard\/dist\/order-detail-[^/]+\.mjs$/.test(filename)
  if (!form && !detail) return
  let code = source
  const replace = (before: string, after: string) => {
    if (!code.includes(before)) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, `Device fulfillment: dashboard integration changed in ${path.basename(filename)} (${before.slice(0, 60)})`)
    code = code.replace(before, after)
  }
  const component = (relative: string) => JSON.stringify(path.resolve(__dirname, relative).replace(/\\/g, "/"))
  if (form) {
    replace('const { reservations } = useReservationItems({', 'const deviceFulfillment = useDeviceFulfillment(order.id, createOrderFulfillment);\n  const { reservations } = useReservationItems({')
    replace('await createOrderFulfillment(payload);', 'await deviceFulfillment.create(payload);')
    replace('/* @__PURE__ */ jsx2("div", { className: "mt-8 pt-8 ",', 'deviceExtensionJsx(DeviceFulfillmentPicker, { state: deviceFulfillment, locationId: selectedLocationId, locationName: stockLocations.options.find((location) => location.value === selectedLocationId)?.label, quantities: form.watch("quantity"), itemIds: fulfillableItems.filter((item) => !requiresShipping || selectedShippingOption?.shipping_profile_id === item.variant?.product?.shipping_profile?.id).map((item) => item.id) }),\n          /* @__PURE__ */ jsx2("div", { className: "mt-8 pt-8 ",')
    replace('isLoading: isMutating,', 'isLoading: isMutating || deviceFulfillment.busy,')
    replace('disabled: !selectedShippingOptionId', 'disabled: !selectedShippingOptionId || deviceFulfillment.busy || !deviceFulfillment.query.data')
    return `import { jsx as deviceExtensionJsx } from "react/jsx-runtime"\nimport { useDeviceFulfillment, DeviceFulfillmentPicker } from ${component("../admin/components/device-fulfillment.tsx")}\n${code}`
  }
  replace('var UnfulfilledItem = ({\n  item,\n  currencyCode', 'var UnfulfilledItem = ({\n  orderId,\n  item,\n  currencyCode')
  replace('item,\n        currencyCode: order.currency_code', 'item,\n        orderId: order.id,\n        currencyCode: order.currency_code')
  const options = '/* @__PURE__ */ jsx11(Text8, { size: "small", children: item.variant?.options?.map((o) => o.value).join(" \\xB7 ") })'
  replace(options, `${options}, deviceExtensionJsx(OrderItemDevices, { orderId, itemId: item.id })`)
  const product = `/* @__PURE__ */ jsx11("li", { children: /* @__PURE__ */ jsxs12(Text8, { size: "small", leading: "compact", children: [
        f_item.quantity,
        "x ",
        f_item.title
      ] }) }, f_item.line_item_id)`
  replace(product, `deviceExtensionJsx("li", { children: [deviceExtensionJsx(Text8, { size: "small", children: f_item.quantity + "x " + f_item.title }), deviceExtensionJsx(OrderItemDevices, { orderId: order.id, itemId: f_item.line_item_id, fulfillmentId: fulfillment.id })] }, f_item.line_item_id)`)

  return `import { jsx as deviceExtensionJsx } from "react/jsx-runtime"\nimport { OrderItemDevices } from ${component("../admin/widgets/order-devices.tsx")}\n${transformOrderCancellation(code)}`
}


export const adminDeviceFulfillment = (): Plugin => ({
  name: "admin-device-fulfillment",
  enforce: "pre",
  transform(source, id) {
    const code = transformDeviceFulfillment(source, id)
    if (code) return { code, map: null }
  },
})

type OptimizationPlugin = NonNullable<NonNullable<DepOptimizationOptions["esbuildOptions"]>["plugins"]>[number]
export const adminDeviceFulfillmentOptimizer = (): OptimizationPlugin => ({
  name: "admin-device-fulfillment-order-cancellation-optimizer",
  setup(build) {
    // Keep workspace components in Vite's pipeline so asset ?url imports and HMR work.
    build.onResolve({ filter: /\/src\/admin\/(?:components\/(?:device-fulfillment|order-cancellation)|widgets\/order-devices)\.tsx$/ }, (args) => ({
      path: `/@fs/${args.path}`,
      external: true,
    }))
    build.onLoad({ filter: /order-(?:create-fulfillment|detail)-[^/\\]+\.mjs$/ }, (args) => {
      const contents = transformDeviceFulfillment(fs.readFileSync(args.path, "utf8"), args.path)
      if (contents) return { contents, loader: "js", resolveDir: path.dirname(args.path) }
    })
  },
})
