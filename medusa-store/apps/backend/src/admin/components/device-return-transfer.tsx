import { Button, Drawer, Label, Text, Textarea, toast } from "@medusajs/ui"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { sdk } from "../lib/sdk"
import { notifyError } from "../lib/notifications"
import { localizeDeviceMessage } from "../lib/device-messages"
import { useDeviceTranslations } from "../lib/use-device-translations"

export default function DeviceReturnTransfer({ device, onClose }: {
  device: { id: string; serial_number: string | null }
  onClose: () => void
}) {
  const { t } = useDeviceTranslations()
  const client = useQueryClient()
  const [notes, setNotes] = useState("")
  const mutation = useMutation({
    mutationFn: () => sdk.client.fetch("/admin/devices", { method: "POST", body: { action: "transfer_return", device_id: device.id, notes: notes.trim() } }),
    onSuccess: async () => {
      await Promise.all(["device-lookup", "device-details", "devices", "order-devices", "order-device-candidates"].map((key) => client.invalidateQueries({ queryKey: [key] })))
      toast.success(t("deviceScan.returnTransfer.saved"))
      onClose()
    },
    onError: (error: Error) => notifyError(localizeDeviceMessage(error.message || t("deviceScan.operationError"), t)),
  })
  const busy = mutation.isPending
  return <Drawer open onOpenChange={(open) => { if (!open && !busy) onClose() }}>
    <Drawer.Content aria-describedby={undefined} onEscapeKeyDown={(event) => { if (busy) event.preventDefault() }}>
      <Drawer.Header><Drawer.Title>{t("deviceScan.returnTransfer.title")}</Drawer.Title></Drawer.Header>
      <Drawer.Body className="space-y-4 overflow-y-auto px-6 py-4">
        <Text size="small" weight="plus">{device.serial_number || device.id}</Text>
        <Text size="small" className="text-ui-fg-subtle">{t("deviceScan.returnTransfer.help")}</Text>
        <form id="device-return-transfer" onSubmit={(event) => { event.preventDefault(); if (notes.trim() && !busy) mutation.mutate() }}>
          <Label className="block space-y-2">
            <Text size="small" weight="plus">{t("deviceScan.returnTransfer.notes")}</Text>
            <Textarea required maxLength={2000} disabled={busy} aria-label={t("deviceScan.returnTransfer.notes")} value={notes} onChange={(event) => setNotes(event.target.value)} />
          </Label>
        </form>
      </Drawer.Body>
      <Drawer.Footer>
        <Button size="small" variant="secondary" disabled={busy} onClick={onClose}>{t("deviceScan.editCancel")}</Button>
        <Button size="small" type="submit" form="device-return-transfer" disabled={busy || !notes.trim()} isLoading={busy}>{t("deviceScan.returnTransfer.confirm")}</Button>
      </Drawer.Footer>
    </Drawer.Content>
  </Drawer>
}
