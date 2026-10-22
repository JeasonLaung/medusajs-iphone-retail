import type { MedusaContainer } from "@medusajs/framework/types"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { AFTER_SALES_MODULE } from "../after-sales"
import type AfterSalesModuleService from "../after-sales/service"
import { webhookEventLabels, type WebhookEvent } from "../../lib/webhook-events"

export type WebhookMessage = {
  title: string
  language: "zh" | "en"
  lines: string[]
  links: { label: string; url: string }[]
}
export type WebhookEventData = {
  id?: string; order_id?: string; fulfillment_id?: string
  return_id?: string; exchange_id?: string; claim_id?: string
}
const resourceFields: Partial<Record<WebhookEvent, keyof WebhookEventData>> = {
  "order.fulfillment_created": "fulfillment_id", "order.fulfillment_canceled": "fulfillment_id",
  "order.return_requested": "return_id", "order.return_received": "return_id",
  "order.exchange_created": "exchange_id", "order.claim_created": "claim_id",
}
export function webhookResourceId(event: WebhookEvent, data: WebhookEventData) {
  return data[resourceFields[event] || "id"]
}

type Transaction = { amount: unknown }
type Payment = {
  id: string; payment_collection_id: string; currency_code: string; amount: unknown
  captures?: Transaction[]; refunds?: Transaction[]
}
type Item = {
  id: string; title?: string | null; variant_title?: string | null; variant_sku?: string | null
  variant?: { sku?: string | null } | null; quantity?: unknown; unit_price?: unknown
}
type Fulfillment = { id: string; labels?: { tracking_number?: string | null; tracking_url?: string | null }[] | null }
type Order = {
  id: string; display_id?: number; custom_display_id?: string | null; email?: string | null
  created_at?: string | Date; currency_code: string; total?: unknown; status?: string
  payment_status?: string; fulfillment_status?: string; items?: Item[] | null
  shipping_address?: Record<string, string | null> | null
  fulfillments?: Fulfillment[] | null
  payment_collections?: { captured_amount?: unknown; refunded_amount?: unknown }[] | null
}
type AfterSales = {
  id: string; order_id: string; type: string; status: string; reason: string
  created_at?: string | Date; items: { lines?: { item_id: string; quantity: number }[] }
}
export type WebhookContext = { resourceId: string; order?: Order; payment?: Payment; afterSales?: AfterSales }

export const notificationOrderFields = [
  "id", "display_id", "custom_display_id", "email", "created_at", "currency_code", "total", "status", "payment_status", "fulfillment_status",
  "shipping_address.first_name", "shipping_address.last_name", "shipping_address.company", "shipping_address.address_1", "shipping_address.address_2",
  "shipping_address.city", "shipping_address.province", "shipping_address.postal_code", "shipping_address.country_code",
  "items.id", "items.title", "items.variant_title", "items.variant_sku", "items.variant.sku", "items.quantity", "items.unit_price",
  "payment_collections.captured_amount", "payment_collections.refunded_amount",
  "fulfillments.id", "fulfillments.labels.tracking_number", "fulfillments.labels.tracking_url",
]

export async function loadWebhookContext(container: MedusaContainer, event: WebhookEvent, data: WebhookEventData): Promise<WebhookContext> {
  const resourceId = webhookResourceId(event, data)
  if (!resourceId) throw new Error("Webhook event has no resource identifier")
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const context: WebhookContext = { resourceId }
  let orderId = data.order_id
  if (event.startsWith("after_sales.")) {
    const service: AfterSalesModuleService = container.resolve(AFTER_SALES_MODULE)
    context.afterSales = await service.retrieveAfterSalesRequest(resourceId) as unknown as AfterSales
    // A queued event may be processed after another status change.
    context.afterSales = { ...context.afterSales, status: event === "after_sales.created" ? "pending" : event.slice("after_sales.".length) }
    orderId = context.afterSales.order_id
  } else if (event.startsWith("payment.")) {
    const { data: payments } = await query.graph({
      entity: "payment", filters: { id: resourceId },
      fields: ["id", "payment_collection_id", "currency_code", "amount", "captures.amount", "refunds.amount"],
    })
    if (!payments[0]) throw new Error("Webhook payment is unavailable")
    context.payment = payments[0] as unknown as Payment
    const { data: links } = await query.graph({
      entity: "order_payment_collection", fields: ["order.id"],
      filters: { payment_collection_id: context.payment.payment_collection_id },
    })
    orderId = links[0]?.order?.id
  } else if (event === "shipment.created" || event === "delivery.created") {
    const { data: links } = await query.graph({
      entity: "order_fulfillment", fields: ["order.id"], filters: { fulfillment_id: resourceId },
    })
    orderId = links[0]?.order?.id
  } else if (!orderId) orderId = resourceId
  if (orderId) {
    const { data: orders } = await query.graph({ entity: "order", fields: notificationOrderFields, filters: { id: orderId } })
    if (!orders[0]) throw new Error("Webhook order is unavailable")
    context.order = orders[0] as unknown as Order
  }
  return context
}

function clean(value: unknown, limit = 400) {
  return String(value ?? "").replace(/[\r\n\t]+/g, " ").trim().slice(0, limit)
}
function sum(values: unknown[]) { return values.reduce<number>((total, value) => total + (Number(value) || 0), 0) }
function money(value: unknown, currency: string, language: "zh" | "en") {
  const amount = Number(value)
  if (value == null || !Number.isFinite(amount)) return "—"
  try { return new Intl.NumberFormat(language === "zh" ? "zh-CN" : "en-GB", { style: "currency", currency: currency.toUpperCase() }).format(amount) }
  catch { return `${amount.toFixed(2)} ${clean(currency).toUpperCase()}` }
}
function date(value: string | Date | undefined, language: "zh" | "en") {
  if (!value || !Number.isFinite(new Date(value).getTime())) return "—"
  return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-GB", {
    timeZone: process.env.WEBHOOK_TIME_ZONE || "Asia/Shanghai", dateStyle: "medium", timeStyle: "long",
  }).format(new Date(value))
}
export function webhookAdminLink(orderId?: string) {
  const url = new URL(process.env.WEBHOOK_ADMIN_URL || process.env.MEDUSA_BACKEND_URL || "http://localhost:9001")
  if (!["http:", "https:"].includes(url.protocol) || url.username || url.password) throw new Error("Invalid Webhook Admin URL")
  url.search = ""
  url.hash = ""
  const root = url.pathname.replace(/\/$/, "").replace(/\/app$/, "")
  url.pathname = `${root}/app/orders${orderId ? `/${encodeURIComponent(orderId)}` : ""}`
  return url.toString()
}

const states: Record<string, readonly [string, string]> = {
  pending: ["待处理", "Pending"], completed: ["已完成", "Completed"], canceled: ["已取消", "Canceled"], archived: ["已归档", "Archived"], draft: ["草稿", "Draft"],
  processing: ["处理中", "Processing"], rejected: ["已拒绝", "Rejected"], return: ["退货退款", "Return / refund"], exchange: ["换货", "Exchange"], repair: ["维修", "Repair"],
  not_paid: ["未付款", "Not paid"], awaiting: ["待付款", "Awaiting payment"], authorized: ["已授权", "Authorized"], partially_authorized: ["部分授权", "Partially authorized"],
  captured: ["已收款", "Captured"], partially_captured: ["部分收款", "Partially captured"], refunded: ["已退款", "Refunded"], partially_refunded: ["部分退款", "Partially refunded"],
  requires_action: ["需进一步操作", "Requires action"], not_fulfilled: ["未履约", "Not fulfilled"], fulfilled: ["已履约", "Fulfilled"], partially_fulfilled: ["部分履约", "Partially fulfilled"],
  shipped: ["已发货", "Shipped"], partially_shipped: ["部分发货", "Partially shipped"], delivered: ["已送达", "Delivered"], partially_delivered: ["部分送达", "Partially delivered"],
}

export function formatWebhookMessage(context: WebhookContext, event: WebhookEvent, language: "zh" | "en", keyword = ""): WebhookMessage {
  const { order, payment, afterSales, resourceId } = context
  const zh = language === "zh"
  const label = (cn: string, en: string) => zh ? cn : en
  const state = (value: string | undefined) => value ? states[value]?.[zh ? 0 : 1] || clean(value) : "—"
  const number = order ? clean(order.custom_display_id || order.display_id || order.id) : ""
  const title = `${webhookEventLabels[event][zh ? 0 : 1]}${number ? ` · #${number}` : ""}`
  const lines: string[] = keyword ? [clean(keyword, 100)] : []
  const field = (name: string, value: string) => lines.push(`${name}: ${value}`)
  if (order) {
    field(label("订单号", "Order number"), `#${number}`)
    field(label("下单时间", "Placed at"), date(order.created_at, language))
    field(label("邮箱", "Email"), clean(order.email) || "—")
    const address = order.shipping_address
    if (address) {
      const recipient = [address.first_name, address.last_name].filter(Boolean).map(v => clean(v)).join(" ")
      field(label("收货人", "Recipient"), recipient || "—")
      field(label("收货地址", "Shipping address"), [address.company, address.address_1, address.address_2, address.city, address.province, address.postal_code, address.country_code?.toUpperCase()].filter(Boolean).map(v => clean(v)).join(", ") || "—")
    } else field(label("收货地址", "Shipping address"), "—")
    field(label("订单金额", "Order total"), money(order.total, order.currency_code, language))
    field(label("累计实收", "Total captured"), money(sum((order.payment_collections || []).map(p => p.captured_amount)), order.currency_code, language))
    field(label("累计退款", "Total refunded"), money(sum((order.payment_collections || []).map(p => p.refunded_amount)), order.currency_code, language))
    field(label("订单状态", "Order status"), state(order.status))
    field(label("支付状态", "Payment status"), state(order.payment_status))
    field(label("履约状态", "Fulfillment status"), state(order.fulfillment_status))
    lines.push(label("商品明细：", "Items:"))
    const items = order.items || []
    for (const item of items.slice(0, 12)) {
      const sku = clean(item.variant_sku || item.variant?.sku) || "—"
      lines.push(`${clean(item.title)}${item.variant_title ? ` / ${clean(item.variant_title)}` : ""} | SKU: ${sku} × ${clean(item.quantity)} | ${label("单价", "Unit price")}: ${money(item.unit_price, order.currency_code, language)}`)
    }
    if (items.length > 12) lines.push(label(`其余 ${items.length - 12} 项请查看订单。`, `View the order for ${items.length - 12} more items.`))
  }
  if (payment) {
    field(label("支付单 ID", "Payment ID"), clean(payment.id))
    field(label("支付单金额", "Payment amount"), money(payment.amount, payment.currency_code, language))
    field(label("本支付累计实收", "Captured on this payment"), money(sum((payment.captures || []).map(c => c.amount)), payment.currency_code, language))
    field(label("本支付累计退款", "Refunded on this payment"), money(sum((payment.refunds || []).map(r => r.amount)), payment.currency_code, language))
  }
  if (afterSales) {
    field(label("售后单 ID", "After-sales ID"), clean(afterSales.id))
    field(label("售后类型", "Request type"), state(afterSales.type))
    field(label("售后状态", "Request status"), state(afterSales.status))
    field(label("申请时间", "Requested at"), date(afterSales.created_at, language))
    field(label("申请原因", "Reason"), clean(afterSales.reason, 600) || "—")
    for (const item of (afterSales.items.lines || []).slice(0, 12)) {
      const line = order?.items?.find(i => i.id === item.item_id)
      field(label("售后商品", "Requested item"), `${clean(line?.title || item.item_id)} | SKU: ${clean(line?.variant_sku || line?.variant?.sku) || "—"} × ${item.quantity}`)
    }
  }
  if (!payment && !afterSales && resourceId !== order?.id) field(label("业务 ID", "Resource ID"), clean(resourceId))
  const links = [{ label: order ? label("查看订单 / 处理售后", "View order / manage after-sales") : label("查看订单列表", "View orders"), url: webhookAdminLink(order?.id) }]
  const fulfillment = order?.fulfillments?.find(f => f.id === resourceId)
  for (const tracking of (fulfillment?.labels || []).slice(0, 3)) {
    if (tracking.tracking_number) field(label("物流单号", "Tracking number"), clean(tracking.tracking_number))
    if (tracking.tracking_url) {
      try { const url = new URL(tracking.tracking_url); if (["http:", "https:"].includes(url.protocol) && !url.username && !url.password) links.push({ label: label("查看物流", "Track shipment"), url: url.toString() }) } catch { /* Ignore invalid carrier links. */ }
    }
  }
  return { title, language, lines, links }
}

export function webhookMessageText(message: WebhookMessage) {
  return [message.title, ...message.lines, ...message.links.map(link => `${link.label}: ${link.url}`)].join("\n")
}
