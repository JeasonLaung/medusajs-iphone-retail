import { useEffect, useRef, useState } from "react"
import { Button, Text } from "@medusajs/ui"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import type { HttpTypes } from "@medusajs/framework/types"
import { sdk } from "../lib/sdk"
import type { OrderDeviceData } from "../lib/order-devices"
import { useDeviceTranslations } from "../lib/use-device-translations"
import { notifyError } from "../lib/notifications"
import { localizeDeviceMessage } from "../lib/device-messages"
import { isOrderPaidForAllocation } from "../../lib/order-device-payment"
import OrderDeviceAllocation from "./order-device-allocation"

type Selection = { item_id: string; device_id: string; assignment_id?: string }

export function useDeviceFulfillment(orderId: string, nativeCreate: (payload: HttpTypes.AdminCreateOrderFulfillment) => Promise<unknown>) {
  const { t } = useDeviceTranslations()
  const client = useQueryClient()
  const [selected, setSelected] = useState<Selection[]>([])
  const [busy, setBusy] = useState(false)
  const query = useQuery<OrderDeviceData>({
    queryKey: ["order-devices", orderId],
    queryFn: () => sdk.client.fetch("/admin/devices", { query: { order_id: orderId } }),
  })
  const initializedOrder = useRef<string | null>(null)
  useEffect(() => {
    if (query.data && initializedOrder.current !== orderId) {
      initializedOrder.current = orderId
      setSelected(query.data.assignments.filter((a) => a.status === "reserved").map((a) => ({ item_id: a.item_id, device_id: a.device_id, assignment_id: a.id })))
    }
  }, [query.data, orderId])
  const create = async (payload: HttpTypes.AdminCreateOrderFulfillment) => {
    if (!query.data || query.isError) throw new Error(t("deviceScan.deviceLoadError"))
    const managed = query.data.assignments.some((assignment) => ["reserved", "packed", "shipped"].includes(assignment.status)) || query.data.managed_variants.some((variant) => query.data?.order?.items.some((item) => item.variant_id === variant.variant_id))
    if (!managed) return nativeCreate(payload)
    setBusy(true)
    try {
      await sdk.client.fetch("/admin/devices", { method: "POST", body: {
        action: "create_fulfillment", order_id: orderId, allocations: selected.filter((entry) => payload.items.some((item) => item.id === entry.item_id)),
        fulfillment: { ...payload, no_notification: payload.no_notification ?? false },
      } })
      await Promise.all([
        client.invalidateQueries({ queryKey: ["order-devices", orderId] }),
        client.invalidateQueries({ queryKey: ["order-device-candidates", orderId] }),
        client.invalidateQueries({ queryKey: ["orders"] }),
      ])
    } catch (error) {
      throw new Error(localizeDeviceMessage(error instanceof Error ? error.message : String(error), t))
    } finally { setBusy(false) }
  }
  return { orderId, query, selected, setSelected, busy, create }
}

type State = ReturnType<typeof useDeviceFulfillment>
export function DeviceFulfillmentPicker({ state, locationId, locationName, quantities, itemIds }: {
  state: State; locationId?: string; locationName?: string; quantities: Record<string, number>; itemIds: string[]
}) {
  const { t } = useDeviceTranslations()
  const [devices, setDevices] = useState<OrderDeviceData["devices"]>([])
  const data = state.query.data
  if (!data) return <Text size="small">{t(state.query.isError ? "deviceScan.deviceLoadError" : "deviceScan.loading")}</Text>
  const items = data.order?.items.filter((item) => itemIds.includes(item.id) && Number(quantities[item.id]) > 0 && data.managed_variants.some((variant) => variant.variant_id === item.variant_id)) || []
  if (!items.length) return null
  const selectedDevices = [...devices, ...data.assigned_devices]
  const selectionData: OrderDeviceData = {
    ...data,
    order: data.order && { ...data.order, fulfillments: [], items: items.map((item) => ({ ...item, quantity: Number(quantities[item.id]), detail: { quantity: Number(quantities[item.id]), fulfilled_quantity: 0 } })) },
    assignments: state.selected.map((entry, index) => ({ ...entry, id: `draft-${index}`, source_id: "", status: "reserved", fulfillment_id: null, tracking_number: null, replacement_of: null })),
  }
  return <div className="space-y-3 py-8">
    <Text size="small" weight="plus">{t("orderDevices.dialogTitle")}</Text>
    <Text size="small">{t("orderDevices.fulfillmentAllocationHelp")}</Text>
    {locationId && isOrderPaidForAllocation(data.order || {}) && <OrderDeviceAllocation key={locationId + items.map((item) => item.id).join()} orderId={state.orderId} data={selectionData} busy={state.busy} selectionMode locationId={locationId} locationName={locationName} run={async (command) => {
      if (command.action !== "assign") return false
      try {
      const result = await sdk.client.fetch<OrderDeviceData>("/admin/devices", { query: { device_id: command.device_id } })
      setDevices((current) => [...current.filter((device) => device.id !== command.device_id), ...result.devices])
      state.setSelected((current) => current.some((entry) => entry.device_id === command.device_id) ? current : [...current, { item_id: command.item_id, device_id: command.device_id }])
      return true
      } catch (error) {
        notifyError(localizeDeviceMessage(error instanceof Error ? error.message : String(error), t))
        return false
      }
    }} />}
    {!locationId && <Text size="small">{t("orderDevices.chooseLocationFirst")}</Text>}
    {!isOrderPaidForAllocation(data.order || {}) && <Text size="small">{t("orderDevices.paymentRequired")}</Text>}
    {state.selected.filter((entry) => itemIds.includes(entry.item_id)).map((entry) => {
      const device = selectedDevices.find((candidate) => candidate.id === entry.device_id)
      return <div key={entry.device_id} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3">
        <div className="min-w-0 break-all">
          <Text size="small">{data.order?.items.find((item) => item.id === entry.item_id)?.title} · {device?.serial_number || entry.device_id}</Text>
          {device?.imei && <Text size="small">{t("orderDevices.imei")}: {device.imei}</Text>}
          {device && <Text size="small">{t("orderDevices.autoShippingSource", { name: data.sources.find((source) => source.id === device.source_id)?.name || device.source_id })}</Text>}
        </div>
        <Button type="button" size="small" variant="secondary" disabled={state.busy} onClick={() => state.setSelected((current) => current.filter((candidate) => candidate.device_id !== entry.device_id))}>{t("actions.remove")}</Button>
      </div>
    })}
  </div>
}
