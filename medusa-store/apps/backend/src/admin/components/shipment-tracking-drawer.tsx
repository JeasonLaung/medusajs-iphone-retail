import { Button, Drawer, Input, Label, Text, toast } from "@medusajs/ui"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { sdk } from "../lib/sdk"
import { notifyError } from "../lib/notifications"
import type { Shipment } from "./shipment-tracking"

export default function ShipmentTrackingDrawer({ orderId, shipment, onClose }: {
  orderId: string
  shipment: Shipment
  onClose: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [labels, setLabels] = useState(() => shipment.labels?.length ? shipment.labels.map((label) => ({
    id: label.id, tracking_number: label.tracking_number || "", tracking_url: label.tracking_url === "#" ? "" : label.tracking_url || "",
  })) : [{ id: undefined as string | undefined, tracking_number: "", tracking_url: "" }])
  const [invalid, setInvalid] = useState(false)
  const mutation = useMutation({
    mutationFn: () => sdk.client.fetch(`/admin/orders/${encodeURIComponent(orderId)}/fulfillments/${encodeURIComponent(shipment.id)}/tracking`, {
      method: "POST", body: { labels: labels.map((label) => ({ ...label, tracking_number: label.tracking_number.trim(), tracking_url: label.tracking_url.trim() })) },
    }),
    onSuccess: async () => {
      await Promise.all(["orders", "devices", "order-devices", "device-details", "device-lookup"].map((key) => client.invalidateQueries({ queryKey: [key] })))
      toast.success(t("shipmentTracking.saved"))
      onClose()
    },
    onError: (error: Error) => {
      const key = error.message.includes("SHIPMENT_TRACKING_NOT_SHIPPED") ? "notShipped" : error.message.includes("SHIPMENT_TRACKING_STALE_LABELS") ? "staleLabels" : "saveError"
      notifyError(t(`shipmentTracking.${key}`))
    },
  })
  const validUrl = (value: string) => { try { return ["http:", "https:"].includes(new URL(value.trim()).protocol) } catch { return false } }
  return <Drawer open onOpenChange={(open) => { if (!open && !mutation.isPending) onClose() }}>
    <Drawer.Content aria-describedby="shipment-tracking-description" onEscapeKeyDown={(event) => { if (mutation.isPending) event.preventDefault() }}>
      <Drawer.Header><Drawer.Title>{t("shipmentTracking.title")}</Drawer.Title></Drawer.Header>
      <Drawer.Body className="space-y-6 overflow-y-auto">
        <Text id="shipment-tracking-description" size="small" className="text-ui-fg-subtle">{t("shipmentTracking.description")}</Text>
        <form id="shipment-tracking-form" className="space-y-6" onSubmit={(event) => {
          event.preventDefault()
          setInvalid(true)
          if (labels.some((label) => !label.tracking_number.trim() || (label.tracking_url.trim() && !validUrl(label.tracking_url)))) return
          mutation.mutate()
        }}>
          {labels.map((label, index) => <div key={label.id || index} className="space-y-3">
            <Text size="small" weight="plus">{t("shipmentTracking.entry", { number: index + 1 })}</Text>
            <div className="space-y-2">
              <Label htmlFor={`tracking-number-${index}`}>{t("shipmentTracking.number")}</Label>
              <Input id={`tracking-number-${index}`} required maxLength={200} disabled={mutation.isPending} value={label.tracking_number} aria-invalid={invalid && !label.tracking_number.trim()} onChange={(event) => setLabels((previous) => previous.map((entry, i) => i === index ? { ...entry, tracking_number: event.target.value } : entry))} />
              {invalid && !label.tracking_number.trim() && <Text size="small" className="text-ui-fg-error">{t("shipmentTracking.numberRequired")}</Text>}
            </div>
            <div className="space-y-2">
              <Label htmlFor={`tracking-url-${index}`}>{t("shipmentTracking.url")}</Label>
              <Input id={`tracking-url-${index}`} type="url" maxLength={2000} disabled={mutation.isPending} value={label.tracking_url} aria-invalid={invalid && !!label.tracking_url.trim() && !validUrl(label.tracking_url)} onChange={(event) => setLabels((previous) => previous.map((entry, i) => i === index ? { ...entry, tracking_url: event.target.value } : entry))} />
              {invalid && !!label.tracking_url.trim() && !validUrl(label.tracking_url) && <Text size="small" className="text-ui-fg-error">{t("shipmentTracking.invalidUrl")}</Text>}
            </div>
          </div>)}
          <Button type="button" size="small" variant="secondary" disabled={mutation.isPending || labels.length >= 20} onClick={() => setLabels((previous) => [...previous, { id: undefined, tracking_number: "", tracking_url: "" }])}>{t("shipmentTracking.addEntry")}</Button>
        </form>
      </Drawer.Body>
      <Drawer.Footer><Button size="small" variant="secondary" disabled={mutation.isPending} onClick={onClose}>{t("actions.cancel")}</Button><Button size="small" type="submit" form="shipment-tracking-form" isLoading={mutation.isPending} disabled={mutation.isPending}>{t("actions.save")}</Button></Drawer.Footer>
    </Drawer.Content>
  </Drawer>
}
