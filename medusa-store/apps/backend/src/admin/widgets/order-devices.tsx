import { defineWidgetConfig } from "@medusajs/admin-sdk"
import { isOrderPaidForAllocation } from "../../lib/order-device-payment"
import { useState } from "react"
import { Button, Text, toast } from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import OrderDeviceAllocationDialog from "../components/order-device-allocation-dialog"
import { useDeviceTranslations } from "../lib/use-device-translations"
import { localizeDeviceMessage } from "../lib/device-messages"
import { notifyError } from "../lib/notifications"
import type { OrderDeviceCommand, OrderDeviceData, RunOrderDeviceCommand } from "../lib/order-devices"
import { sdk } from "../lib/sdk"

// Rendered inside native product rows by the dashboard extension.
export function OrderItemDevices({ orderId, itemId, fulfillmentId }: { orderId: string; itemId: string; fulfillmentId?: string }) {
  const { t } = useDeviceTranslations()
  const client = useQueryClient()
  const [open, setOpen] = useState(false)
  const { data, isPending, isError } = useQuery<OrderDeviceData>({
    queryKey: ["order-devices", orderId],
    queryFn: () => sdk.client.fetch("/admin/devices", { query: { order_id: orderId } }),
  })
  const mutation = useMutation({
    mutationFn: (body: OrderDeviceCommand) => sdk.client.fetch("/admin/devices", { method: "POST", body }),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ["order-devices", orderId] }),
        client.invalidateQueries({ queryKey: ["order-device-candidates", orderId] }),
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
  const item = data?.order?.items.find((entry) => entry.id === itemId)
  const assignments = data?.assignments.filter((a) => a.item_id === itemId && (fulfillmentId ? a.fulfillment_id === fulfillmentId && ["packed", "shipped"].includes(a.status) : a.status === "reserved")) || []
  if (data && !assignments.length && !data.managed_variants.some((entry) => entry.variant_id === item?.variant_id)) return null
  const fulfillment = data?.order?.fulfillments?.find((f) => f.id === fulfillmentId)
  const paid = isOrderPaidForAllocation(data?.order || {})
  const readOnly = !paid || ["canceled", "draft"].includes(data?.order?.status || "") || Boolean(fulfillment?.shipped_at || fulfillment?.delivered_at || fulfillment?.canceled_at)
  return <div className="mt-2 space-y-2 text-ui-fg-base" data-order-item-devices={itemId}>
    {isPending && <Text size="small">{t("deviceScan.loading")}</Text>}
    {isError && <Button size="small" variant="secondary" onClick={() => client.invalidateQueries({ queryKey: ["order-devices", orderId] })}>{t("orderDevices.retry")}</Button>}
    {data && <Text size="small" weight="plus">{t("orderDevices.boundDevices", { count: assignments.length })}</Text>}
    {assignments.map((a) => {
      const device = data?.assigned_devices.find((d) => d.id === a.device_id)
      return <div key={a.id} className="space-y-1 break-all">
        <Text size="small">{t("orderDevices.serial")}: {device?.serial_number || a.device_id}</Text>
        {device?.imei && <Text size="small">{t("orderDevices.imei")}: {device.imei}</Text>}
        {device?.imei2 && <Text size="small">{t("orderDevices.imei2")}: {device.imei2}</Text>}
        {device?.eid && <Text size="small">{t("orderDevices.eid")}: {device.eid}</Text>}
        <Text size="small">{t("orderDevices.autoShippingSource", { name: data?.sources.find((source) => source.id === a.source_id)?.name || a.source_id })}</Text>
      </div>
    })}
    {data && !readOnly && <Button size="small" variant="secondary" disabled={mutation.isPending} onClick={() => setOpen(true)}>{t(assignments.length ? "orderDevices.replaceAllocation" : "orderDevices.supplementAllocation")}</Button>}
    {open && data && <OrderDeviceAllocationDialog orderId={orderId} itemId={itemId} data={data} fulfillmentId={fulfillmentId} busy={mutation.isPending} readOnly={readOnly} run={run} onClose={() => setOpen(false)} />}
  </div>
}

// Native rows own the entry points; no separate card/footer is needed.
const OrderDevices = () => null
export default OrderDevices
export const config = defineWidgetConfig({ zone: "order.details.after" })
