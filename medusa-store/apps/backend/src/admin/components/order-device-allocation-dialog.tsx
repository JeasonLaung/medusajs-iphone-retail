import { useState } from "react"
import { Button, FocusModal, Heading, Text } from "@medusajs/ui"
import OrderDeviceAllocation from "./order-device-allocation"
import { useDeviceTranslations } from "../lib/use-device-translations"
import type { OrderDeviceData, RunOrderDeviceCommand } from "../lib/order-devices"
import { sdk } from "../lib/sdk"

type Selection = { item_id: string; device_id: string; assignment_id?: string; fulfillment_id?: string }

export default function OrderDeviceAllocationDialog({ orderId, data, fulfillmentId, busy, readOnly, run, onClose }: {
  orderId: string; data: OrderDeviceData; fulfillmentId?: string; busy: boolean; readOnly: boolean
  run: RunOrderDeviceCommand; onClose: () => void
}) {
  const { t } = useDeviceTranslations()
  const initial = data.assignments.filter((a) => fulfillmentId ? a.fulfillment_id === fulfillmentId && ["packed", "shipped"].includes(a.status) : a.status === "reserved")
  const [selected, setSelected] = useState<Selection[]>(initial.map((a) => ({ item_id: a.item_id, device_id: a.device_id, assignment_id: a.id, ...(fulfillmentId ? { fulfillment_id: fulfillmentId } : {}) })))
  const [draftDevices, setDraftDevices] = useState(data.assigned_devices)
  const missingReplacement = initial.some((a) => !selected.some((entry) => entry.assignment_id === a.id))
  const selectionData: OrderDeviceData = {
    ...data,
    assignments: [
      ...data.assignments.filter((a) => !initial.some((entry) => entry.id === a.id)),
      ...selected.map((entry, index) => ({ id: `draft-${index}`, ...entry, source_id: "", status: fulfillmentId ? "packed" : "reserved", fulfillment_id: fulfillmentId || null, tracking_number: null, replacement_of: null })),
    ],
    order: data.order && { ...data.order, items: data.order.items.filter((item) => !fulfillmentId || data.order?.fulfillments?.find((f) => f.id === fulfillmentId)?.items?.some((entry) => entry.line_item_id === item.id)) },
  }
  const select: RunOrderDeviceCommand = async (body) => {
    if (body.action !== "assign" || selected.some((entry) => entry.device_id === body.device_id)) return false
    const replacing = initial.find((a) => a.item_id === body.item_id && !selected.some((entry) => entry.assignment_id === a.id))
    setSelected((entries) => entries.some((entry) => entry.device_id === body.device_id) ? entries : [...entries, { item_id: body.item_id, device_id: body.device_id, ...(replacing ? { assignment_id: replacing.id } : {}), ...(fulfillmentId ? { fulfillment_id: fulfillmentId } : {}) }])
    // Keep the selected device's identity visible after changing the search/page.
    const candidates = await sdk.client.fetch<OrderDeviceData>("/admin/devices", { query: { device_id: body.device_id } }).catch(() => null)
    if (candidates) setDraftDevices((devices) => [...devices.filter((d) => d.id !== body.device_id), ...candidates.devices])
    return true
  }
  return <FocusModal open onOpenChange={(open) => { if (!open && !busy) onClose() }}>
    <FocusModal.Content>
      <FocusModal.Header><Heading level="h2">{t("orderDevices.dialogTitle")}</Heading></FocusModal.Header>
      <FocusModal.Body className="overflow-y-auto">
        <div className="mx-auto w-full max-w-3xl space-y-4 p-6">
          <Text size="small">{t("orderDevices.dialogHelp")}</Text>
          {!readOnly && <OrderDeviceAllocation orderId={orderId} data={selectionData} busy={busy} run={select} fulfillmentId={fulfillmentId} selectionMode />}
          <Heading level="h3">{t("orderDevices.selectedDevices", { count: selected.length })}</Heading>
          {selected.map((entry) => {
            const device = draftDevices.find((d) => d.id === entry.device_id)
            return <div key={entry.device_id} className="flex items-center justify-between gap-3 rounded-lg border p-3">
              <div className="space-y-1">
                <Text size="small" weight="plus">{device?.serial_number || entry.device_id}</Text>
                {device?.imei && <Text size="small">{t("orderDevices.imei")}: {device.imei}</Text>}
                {device?.imei2 && <Text size="small">{t("orderDevices.imei2")}: {device.imei2}</Text>}
                {device?.eid && <Text size="small">{t("orderDevices.eid")}: {device.eid}</Text>}
                {device && <Text size="small">{t("orderDevices.autoShippingSource", { name: data.sources.find((source) => source.id === device.source_id)?.name || device.source_id })}</Text>}
              </div>
              {!readOnly && <Button size="small" variant="secondary" disabled={busy} onClick={() => setSelected((entries) => entries.filter((item) => item.device_id !== entry.device_id))}>{t(entry.assignment_id ? "orderDevices.chooseReplacement" : "actions.remove")}</Button>}
            </div>
          })}
          {missingReplacement && <Text size="small">{t("orderDevices.replacementRequired")}</Text>}
        </div>
      </FocusModal.Body>
      <FocusModal.Footer>
        <Button size="small" variant="secondary" disabled={busy} onClick={onClose}>{t("actions.cancel")}</Button>
        {!readOnly && <Button size="small" isLoading={busy} disabled={busy || !selected.length || missingReplacement} onClick={async () => {
          if (await run({ action: "save_allocation", order_id: orderId, allocations: selected })) onClose()
        }}>{t("orderDevices.confirmAllocation")}</Button>}
      </FocusModal.Footer>
    </FocusModal.Content>
  </FocusModal>
}
