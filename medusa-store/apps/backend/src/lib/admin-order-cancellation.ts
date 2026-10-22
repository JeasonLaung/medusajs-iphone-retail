import path from "node:path"
import { MedusaError } from "@medusajs/framework/utils"

// Extend the native action and its activity row, keeping the dashboard layout.
export function transformOrderCancellation(source: string) {
  let code = source
  const replace = (before: string, after: string) => {
    if (!code.includes(before)) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, "Order cancellation: dashboard integration changed")
    code = code.replace(before, after)
  }
  const start = code.indexOf("var OrderGeneralSection = ({ order }) => {")
  const end = code.indexOf("var FulfillmentBadge =", start)
  const section = code.slice(start, end)
  const action = /  const \{ mutateAsync: cancelOrder \} = useCancelOrder\(order.id\);[\s\S]*?\n  };\n  return/
  if (start < 0 || end < 0 || !action.test(section)) {
    throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, "Order cancellation: dashboard action changed")
  }
  code = code.slice(0, start) + section.replace(action, "  const { open: handleCancel, dialog: cancellationDialog } = useOrderCancellation(order);\n  return") + code.slice(end)
  replace('return /* @__PURE__ */ jsxs13(Container8, { className: "flex items-center justify-between px-6 py-4", children: [', 'return /* @__PURE__ */ jsxs13(Container8, { className: "flex items-center justify-between px-6 py-4", children: [cancellationDialog,')
  replace('title: t("orders.activity.events.canceled.title"),\n        timestamp: order.canceled_at', 'title: t("orders.activity.events.canceled.title"),\n        children: cancellationJsx(OrderCancellationReason, { cancellation: order.metadata?.cancellation }),\n        timestamp: order.canceled_at')
  const component = path.resolve(__dirname, "../admin/components/order-cancellation.tsx").replace(/\\/g, "/")
  return `import { jsx as cancellationJsx } from "react/jsx-runtime"\nimport { useOrderCancellation, OrderCancellationReason } from ${JSON.stringify(component)}\n${code}`
}
