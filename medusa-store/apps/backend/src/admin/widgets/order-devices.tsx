import { useEffect, useState } from "react"
import { createPortal } from "react-dom"
import { defineWidgetConfig } from "@medusajs/admin-sdk"
import type { AdminOrder, DetailWidgetProps } from "@medusajs/framework/types"
import { Button, Container, Text, toast } from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import OrderDeviceAllocationDialog from "../components/order-device-allocation-dialog"
import { useDeviceTranslations } from "../lib/use-device-translations"
import { localizeDeviceMessage } from "../lib/device-messages"
import { notifyError } from "../lib/notifications"
import type { OrderDeviceCommand, OrderDeviceData, RunOrderDeviceCommand } from "../lib/order-devices"
import { sdk } from "../lib/sdk"

type Host = { element: HTMLElement; fulfillmentId?: string }

// Medusa exposes an order widget zone, but no fulfillment-specific zone.
// Mount our controls into the native fulfillment cards without changing vendor files.
function useFulfillmentHosts(order: AdminOrder) {
  const { t, i18n } = useDeviceTranslations()
  const [hosts, setHosts] = useState<Host[]>([])
  useEffect(() => {
    const owned = new Set<HTMLElement>()
    const sync = () => {
      const headings = [...document.querySelectorAll("h2")]
      const targets = [
        { title: t("orders.fulfillment.unfulfilledItems"), fulfillmentId: undefined as string | undefined },
        ...(order.fulfillments || []).map((f, index) => ({ title: t("orders.fulfillment.number", { number: index + 1 }), fulfillmentId: f.id })),
      ]
      const next: Host[] = []
      for (const target of targets) {
        const heading = headings.find((entry) => entry.textContent?.trim() === target.title)
        const card = heading?.parentElement?.parentElement
        if (!card) continue
        let element = card.querySelector<HTMLElement>("[data-order-allocation-host]")
        if (!element) {
          element = document.createElement("div")
          element.dataset.orderAllocationHost = order.id
          element.className = "space-y-2 px-6 py-4"
          owned.add(element)
        }
        const shippingLabels = [
          t("orders.fulfillment.markAsShipped"),
          t("orders.fulfillment.markAsDelivered"),
          t("orders.fulfillment.markAsPickedUp"),
        ]
        const shippingActions = target.fulfillmentId
          ? [...card.children].find((child) => child !== element && [...child.querySelectorAll("button")].some((button) => shippingLabels.includes(button.textContent?.trim() || "")))
          : undefined
        // Keep allocation before the native shipping footer, including after native rerenders.
        if (shippingActions) {
          if (element.nextElementSibling !== shippingActions) card.insertBefore(element, shippingActions)
        } else if (element.parentElement !== card) {
          card.append(element)
        }
        next.push({ element, fulfillmentId: target.fulfillmentId })
      }
      setHosts((current) => current.length === next.length && current.every((host, index) => host.element === next[index].element && host.fulfillmentId === next[index].fulfillmentId) ? current : next)
    }
    sync()
    const observer = new MutationObserver(sync)
    observer.observe(document.body, { childList: true, subtree: true })
    return () => { observer.disconnect(); owned.forEach((element) => element.remove()) }
  }, [order.id, order.fulfillments, i18n.language, t])
  return hosts
}

const OrderDevices = ({ data: order }: DetailWidgetProps<AdminOrder>) => {
  const { t } = useDeviceTranslations()
  const client = useQueryClient()
  const [dialog, setDialog] = useState<{ fulfillmentId?: string } | null>(null)
  const hosts = useFulfillmentHosts(order)
  const { data, isPending, isError } = useQuery<OrderDeviceData>({
    queryKey: ["order-devices", order.id],
    queryFn: () => sdk.client.fetch("/admin/devices", { query: { order_id: order.id } }),
  })
  const mutation = useMutation({
    mutationFn: (body: OrderDeviceCommand) => sdk.client.fetch("/admin/devices", { method: "POST", body }),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ["order-devices", order.id] }),
        client.invalidateQueries({ queryKey: ["order-device-candidates", order.id] }),
        client.invalidateQueries({ queryKey: ["device-lookup"] }),
        client.invalidateQueries({ queryKey: ["orders"] }),
      ])
      toast.success(t("orderDevices.allocationSaved"))
    },
    onError: (error: Error) => notifyError(localizeDeviceMessage(error.message, t)),
  })
  const run: RunOrderDeviceCommand = async (body) => {
    if (mutation.isPending) return false
    try { await mutation.mutateAsync(body); return true } catch { return false }
  }
  const fulfillment = data?.order?.fulfillments?.find((f) => f.id === dialog?.fulfillmentId)
  const readOnly = ["canceled", "draft"].includes(data?.order?.status || order.status) || Boolean(fulfillment?.shipped_at || fulfillment?.delivered_at || fulfillment?.canceled_at)
  const controls = (fulfillmentId?: string) => {
    const assignments = data?.assignments.filter((a) => fulfillmentId ? a.fulfillment_id === fulfillmentId && ["packed", "shipped"].includes(a.status) : a.status === "reserved") || []
    return <>
      {isPending && <Text size="small">{t("deviceScan.loading")}</Text>}
      {isError && <Text size="small">{t("deviceScan.deviceLoadError")}</Text>}
      <Text size="small" weight="plus">{t("orderDevices.boundDevices", { count: assignments.length })}</Text>
      {assignments.map((a) => {
        const device = data?.assigned_devices.find((d) => d.id === a.device_id)
        return <div key={a.id}>
          <Text size="small">{device?.serial_number || a.device_id}{device?.imei ? " · " + device.imei : ""}</Text>
          <Text size="small">{t("orderDevices.autoShippingSource", { name: data?.sources.find((source) => source.id === a.source_id)?.name || a.source_id })}</Text>
        </div>
      })}
      <Button size="small" variant="secondary" disabled={!data || mutation.isPending} onClick={() => setDialog({ fulfillmentId })}>{t("orderDevices.manageAllocation")}</Button>
    </>
  }
  return <>
    {hosts.map((host) => createPortal(controls(host.fulfillmentId), host.element, host.fulfillmentId || "unfulfilled"))}
    {!hosts.length && <Container className="space-y-2">{controls()}</Container>}
    {dialog && data && <OrderDeviceAllocationDialog key={dialog.fulfillmentId || "unfulfilled"} orderId={order.id} data={data} fulfillmentId={dialog.fulfillmentId} busy={mutation.isPending} readOnly={readOnly} run={run} onClose={() => setDialog(null)} />}
  </>
}
export default OrderDevices
export const config = defineWidgetConfig({ zone: "order.details.after" })
