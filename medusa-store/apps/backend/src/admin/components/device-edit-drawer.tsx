import { Button, Drawer, Input, Label, Select, Text, Textarea, toast } from "@medusajs/ui"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import type { ReactNode } from "react"
import type { UpdateDeviceBody } from "../../api/admin/devices/validators"
import { sdk } from "../lib/sdk"
import { notifyError } from "../lib/notifications"
import { localizeDeviceMessage } from "../lib/device-messages"
import { useDeviceTranslations } from "../lib/use-device-translations"
import DevicePhotosInput from "./device-photos-input"
import DeviceVariantSelect from "./device-variant-select"
import type { DeviceVariantChoice } from "./device-variant-select"

type EditableDevice = Omit<UpdateDeviceBody, "action" | "device_id"> & { id: string; serial_number: string | null }
type Source = { id: string; name: string; location_id: string }

export default function DeviceEditDrawer({ device, sources, variants, onClose }: {
  device: EditableDevice; sources: Source[]; variants: DeviceVariantChoice[]; onClose: () => void
}) {
  const { t } = useDeviceTranslations()
  const client = useQueryClient()
  const [form, setForm] = useState(device)
  const [variant, setVariant] = useState(variants.find((entry) => entry.id === device.variant_id))
  const [cost, setCost] = useState((device.cost_cents / 100).toFixed(2))
  const [uploading, setUploading] = useState(false)
  const set = <K extends keyof EditableDevice>(key: K, value: EditableDevice[K]) => setForm((previous) => ({ ...previous, [key]: value }))
  const mutation = useMutation({
    mutationFn: () => {
      const amount = Number(cost)
      if (!cost.trim() || !Number.isFinite(amount) || amount < 0 || amount > 1000000 || Math.abs(amount * 100 - Math.round(amount * 100)) > 0.000001) throw new Error(t("deviceScan.editInvalidCost"))
      const body: UpdateDeviceBody = { action: "update", device_id: device.id,
        variant_id: form.variant_id, source_id: form.source_id, location_id: form.location_id,
        imei: form.imei?.trim() || null, imei2: form.imei2?.trim() || null, eid: form.eid?.trim() || null,
        category: form.category, state: form.state, grade: form.grade, battery_health: form.battery_health,
        cost_cents: Math.round(amount * 100), currency_code: form.currency_code.toLowerCase(),
        bin: form.bin, notes: form.notes, photos: form.photos || [] }
      return sdk.client.fetch("/admin/devices", { method: "POST", body })
    },
    onSuccess: async () => {
      await Promise.all(["device-details", "device-lookup", "devices", "order-devices", "order-device-candidates"].map((key) => client.invalidateQueries({ queryKey: [key] })))
      toast.success(t("deviceScan.editSaved"))
      onClose()
    },
    onError: (error: Error) => notifyError(localizeDeviceMessage(error.message || t("deviceScan.operationError"), t)),
  })
  const busy = mutation.isPending || uploading
  const field = (label: string, children: ReactNode) => <Label className="block space-y-2"><Text size="small" weight="plus">{label}</Text>{children}</Label>
  const select = (key: "source_id" | "location_id" | "state" | "category" | "grade", label: string, options: { value: string; label: string }[]) => field(label,
    <Select value={form[key] || "none"} disabled={busy} onValueChange={(value) => setForm((previous) => ({ ...previous, [key]: value === "none" ? null : value }))}>
      <Select.Trigger aria-label={label}><Select.Value /></Select.Trigger>
      <Select.Content>{options.map((option) => <Select.Item key={option.value} value={option.value}>{option.label}</Select.Item>)}</Select.Content>
    </Select>)
  const stateLabel = (value: string) => t(`deviceScan.${({ repair_ready: "repairReady", supplier_returned: "supplierReturned" } as Record<string, string>)[value] || value}`)
  return <Drawer open onOpenChange={(open) => { if (!open && !busy) onClose() }}>
    <Drawer.Content aria-describedby={undefined} onEscapeKeyDown={(event) => { if (busy) event.preventDefault() }}>
      <Drawer.Header><Drawer.Title>{t("deviceScan.editDevice")}</Drawer.Title></Drawer.Header>
      <Drawer.Body className="space-y-4 overflow-y-auto px-6 py-4">
        <fieldset disabled={busy} className="space-y-4">
          {field(t("orderDevices.serial"), <Input value={device.serial_number || ""} readOnly aria-readonly aria-label={t("orderDevices.serial")} />)}
          <Text size="small">{t("deviceScan.editHelp")}</Text>
          <DeviceVariantSelect value={variant} disabled={busy} onChange={(value) => { setVariant(value); set("variant_id", value?.id || "") }} />
          {field(t("orderDevices.imei"), <Input value={form.imei || ""} inputMode="numeric" aria-label={t("orderDevices.imei")} onChange={(event) => set("imei", event.target.value || null)} />)}
          {field(t("orderDevices.imei2"), <Input value={form.imei2 || ""} inputMode="numeric" aria-label={t("orderDevices.imei2")} onChange={(event) => set("imei2", event.target.value || null)} />)}
          {field(t("orderDevices.eid"), <Input value={form.eid || ""} inputMode="numeric" aria-label={t("orderDevices.eid")} onChange={(event) => set("eid", event.target.value || null)} />)}
          {select("source_id", t("deviceScan.lookupSource"), sources.map((source) => ({ value: source.id, label: source.name })))}
          {select("location_id", t("deviceScan.lookupLocation"), [...new Map(sources.map((source) => [source.location_id, { value: source.location_id, label: source.name }])).values()])}
          {select("category", t("deviceScan.category"), ["sale", "returned", "recycled"].map((value) => ({ value, label: t(`deviceScan.categories.${value}`) })))}
          {select("state", t("deviceScan.lookupState"), [...new Set([form.state, "pending", "inspection", ...(device.state === "pending" ? [] : ["available"]), "repair", "retired", "supplier_returned"])].map((value) => ({ value, label: stateLabel(value) })))}
          {select("grade", t("deviceScan.grade"), [{ value: "none", label: t("orderDevices.notRecorded") }, ...["S", "A", "B", "C"].map((value) => ({ value, label: value }))])}
          {field(t("deviceScan.battery"), <Input type="number" min={0} max={100} step={1} value={form.battery_health ?? ""} aria-label={t("deviceScan.battery")} onChange={(event) => set("battery_health", event.target.value === "" ? null : Number(event.target.value))} />)}
          {field(t("deviceScan.cost"), <Input type="number" min={0} step="0.01" value={cost} aria-label={t("deviceScan.cost")} onChange={(event) => setCost(event.target.value)} />)}
          {field(t("deviceScan.lookupCurrency"), <Input maxLength={3} value={form.currency_code} aria-label={t("deviceScan.lookupCurrency")} onChange={(event) => set("currency_code", event.target.value.toLowerCase())} />)}
          {field(t("deviceScan.bin"), <Input maxLength={200} value={form.bin || ""} aria-label={t("deviceScan.bin")} onChange={(event) => set("bin", event.target.value)} />)}
          {field(t("deviceScan.intakeNotes"), <Textarea maxLength={2000} value={form.notes || ""} aria-label={t("deviceScan.intakeNotes")} onChange={(event) => set("notes", event.target.value)} />)}
        </fieldset>
        <DevicePhotosInput value={form.photos || []} onChange={(value) => set("photos", value)} disabled={mutation.isPending} uploading={uploading} onUploadingChange={setUploading} />
      </Drawer.Body>
      <Drawer.Footer><Button size="small" variant="secondary" disabled={busy} onClick={onClose}>{t("deviceScan.editCancel")}</Button><Button size="small" disabled={busy || !form.variant_id} isLoading={mutation.isPending} onClick={() => mutation.mutate()}>{t("deviceScan.editSave")}</Button></Drawer.Footer>
    </Drawer.Content>
  </Drawer>
}
