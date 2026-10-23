import { Button, Drawer, Input, Label, Select, Text, Textarea, toast } from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useState } from "react"
import type { DeviceCommandBody } from "../../api/admin/devices/validators"
import { sdk } from "../lib/sdk"
import { notifyError } from "../lib/notifications"
import { localizeDeviceMessage } from "../lib/device-messages"
import { useDeviceTranslations } from "../lib/use-device-translations"
import DeviceVariantSelect, { type DeviceVariantChoice } from "./device-variant-select"
import DeviceProcessingHistory, { type ProcessingEvent } from "./device-processing-history"
import DevicePhotosInput, { type DevicePhoto } from "./device-photos-input"

type Device = {
  id: string
  serial_number: string | null
  state: string
  grade: string | null
  battery_health: number | null
}
type Result = Extract<DeviceCommandBody, { action: "inspect" }>["result"]

export default function ReturnedDeviceProcessing({ device, initialVariant, onClose }: {
  device: Device
  initialVariant?: DeviceVariantChoice
  onClose: () => void
}) {
  const { t } = useDeviceTranslations()
  const client = useQueryClient()
  const history = useQuery<{ events: ProcessingEvent[] }>({
    queryKey: ["device-details", device.id],
    queryFn: () => sdk.client.fetch("/admin/devices", { query: { device_id: device.id } }),
  })
  useEffect(() => { if (history.isError) notifyError(t("deviceScan.loadError")) }, [history.isError, t])
  const [result, setResult] = useState<Result>("pass")
  const [variant, setVariant] = useState(initialVariant)
  const [grade, setGrade] = useState(device.grade || "")
  const [battery, setBattery] = useState(device.battery_health == null ? "" : String(device.battery_health))
  const [notes, setNotes] = useState("")
  const [photos, setPhotos] = useState<DevicePhoto[]>([])
  const [uploading, setUploading] = useState(false)
  const batteryHealth = battery.trim() ? Number(battery) : NaN
  const valid = !!variant && ["S", "A", "B", "C"].includes(grade) && Number.isInteger(batteryHealth) && batteryHealth >= 0 && batteryHealth <= 100 && !!notes.trim()
  const mutation = useMutation({
    mutationFn: () => {
      if (!valid) throw new Error(t("deviceScan.returnProcessing.required"))
      const body: DeviceCommandBody = {
        action: "inspect", device_id: device.id, result,
        variant_id: variant!.id, grade: grade as "S" | "A" | "B" | "C",
        battery_health: batteryHealth, notes: notes.trim(), photos,
      }
      return sdk.client.fetch("/admin/devices", { method: "POST", body })
    },
    onSuccess: async () => {
      await Promise.all(["device-lookup", "device-details", "devices", "order-devices", "order-device-candidates", "after-sales"].map((key) => client.invalidateQueries({ queryKey: [key] })))
      toast.success(t("deviceScan.returnProcessing.saved"))
      onClose()
    },
    onError: (error: Error) => notifyError(localizeDeviceMessage(error.message || t("deviceScan.operationError"), t)),
  })
  const busy = mutation.isPending || uploading
  return <Drawer open onOpenChange={(open) => { if (!open && !busy) onClose() }}>
    <Drawer.Content aria-describedby={undefined} onEscapeKeyDown={(event) => { if (busy) event.preventDefault() }}>
      <Drawer.Header><Drawer.Title>{t("deviceScan.returnProcessing.title")}</Drawer.Title></Drawer.Header>
      <Drawer.Body className="space-y-4 overflow-y-auto px-6 py-4">
        <Text size="small" weight="plus">{device.serial_number || device.id}</Text>
        <Text size="small" className="text-ui-fg-subtle">{t("deviceScan.returnProcessing.help")}</Text>
        <form id="returned-device-processing" className="space-y-4" onSubmit={(event) => { event.preventDefault(); if (valid && !busy) mutation.mutate() }}>
          <Label className="block space-y-2">
            <Text size="small" weight="plus">{t("deviceScan.returnProcessing.status")}</Text>
            <Select value={result} disabled={busy} onValueChange={(value) => setResult(value as Result)}>
              <Select.Trigger aria-label={t("deviceScan.returnProcessing.status")}><Select.Value /></Select.Trigger>
              <Select.Content>{(["pass", "repair", "retire", "supplier_return"] as const).map((value) => <Select.Item key={value} value={value}>{t(`deviceScan.returnProcessing.options.${value}`)}</Select.Item>)}</Select.Content>
            </Select>
          </Label>
          <DeviceVariantSelect value={variant} disabled={busy} onChange={setVariant} />
          <Label className="block space-y-2">
            <Text size="small" weight="plus">{t("deviceScan.inspectedGrade")}</Text>
            <Select value={grade || undefined} disabled={busy} onValueChange={setGrade}>
              <Select.Trigger aria-label={t("deviceScan.inspectedGrade")}><Select.Value placeholder={t("deviceScan.inspectedGrade")} /></Select.Trigger>
              <Select.Content>{["S", "A", "B", "C"].map((value) => <Select.Item key={value} value={value}>{value}</Select.Item>)}</Select.Content>
            </Select>
          </Label>
          <Label className="block space-y-2">
            <Text size="small" weight="plus">{t("deviceScan.inspectedBattery")}</Text>
            <Input required type="number" min={0} max={100} step={1} value={battery} disabled={busy} aria-label={t("deviceScan.inspectedBattery")} onChange={(event) => setBattery(event.target.value)} />
          </Label>
          <Label className="block space-y-2">
            <Text size="small" weight="plus">{t("deviceScan.returnProcessing.notes")}</Text>
            <Textarea required maxLength={2000} value={notes} disabled={busy} aria-label={t("deviceScan.returnProcessing.notes")} onChange={(event) => setNotes(event.target.value)} />
          </Label>
          <Text size="small" className="text-ui-fg-subtle">{t("deviceScan.returnProcessing.required")}</Text>
          <DevicePhotosInput label={t("deviceScan.returnProcessing.photos")} value={photos} onChange={setPhotos} disabled={mutation.isPending} uploading={uploading} onUploadingChange={setUploading} />
        </form>
        {history.isPending ? <Text size="small" role="status">{t("deviceScan.loading")}</Text> : history.isError ? <Button size="small" variant="secondary" onClick={() => void history.refetch()}>{t("orderDevices.retry")}</Button> : <DeviceProcessingHistory events={history.data.events} />}
      </Drawer.Body>
      <Drawer.Footer>
        <Button size="small" variant="secondary" disabled={busy} onClick={onClose}>{t("deviceScan.editCancel")}</Button>
        <Button size="small" type="submit" form="returned-device-processing" disabled={!valid || busy} isLoading={busy}>{t("deviceScan.editSave")}</Button>
      </Drawer.Footer>
    </Drawer.Content>
  </Drawer>
}
