import { Button, FocusModal, Heading, Text } from "@medusajs/ui"
import { useQuery } from "@tanstack/react-query"
import { useEffect, useState } from "react"
import { sdk } from "../lib/sdk"
import { notifyError } from "../lib/notifications"
import { useDeviceTranslations } from "../lib/use-device-translations"

import DeviceEditDrawer from "./device-edit-drawer"
import DeviceProcessingHistory from "./device-processing-history"
import RecycledAssessmentSummary from "./recycled-assessment-summary"
import { readAssessment } from "../lib/recycled-assessment"
import type { DeviceVariantChoice } from "./device-variant-select"

type Details = {
  access: { role: string }
  devices: {
    id: string; serial_number: string | null; imei: string | null; imei2: string | null; eid: string | null
    variant_id: string; source_id: string; location_id: string; state: "pending" | "inspection" | "available" | "reserved" | "packed" | "shipped" | "repair" | "repair_ready" | "retired" | "supplier_returned"
    grade: "S" | "A" | "B" | "C" | null; battery_health: number | null; cost_cents: number
    category: "sale" | "returned" | "recycled"; photos: { id: string; url: string }[]; currency_code: string; bin: string; notes: string; created_at: string; updated_at: string
  }[]
  variants: DeviceVariantChoice[]
  sources: { id: string; location_id: string; name: string }[]
  assignments: { id: string; order_id: string; status: string; tracking_number?: string }[]
  events: { id: string; created_at: string; action: string; actor_id: string; actor_name?: string; data: { mobile_intake_nonce?: string; [key: string]: unknown } }[]
}

export default function DeviceDetailsModal({ deviceId, onClose, initialEditing = false }: { deviceId: string; onClose: () => void; initialEditing?: boolean }) {
  const { t, i18n } = useDeviceTranslations()
  const [editing, setEditing] = useState(initialEditing)
  const query = useQuery<Details>({
    queryKey: ["device-details", deviceId],
    queryFn: () => sdk.client.fetch("/admin/devices", { query: { device_id: deviceId } }),
  })
  useEffect(() => {
    if (query.isError) notifyError(t("deviceScan.loadError"))
  }, [query.isError, t])
  const device = query.data?.devices[0]
  const assessment = readAssessment(device?.notes || "")
  const variant = query.data?.variants.find((entry) => entry.id === device?.variant_id)
  const state = (value: string) => t(`deviceScan.${({ repair_ready: "repairReady", supplier_returned: "supplierReturned" } as Record<string, string>)[value] || value}`)
  const date = (value: string) => new Date(value).toLocaleString(i18n.language.startsWith("zh") ? "zh-CN" : "en-GB")
  const fields = device ? [
    [t("orderDevices.serial"), device.serial_number],
    [t("deviceScan.sku"), variant?.sku],
    [t("deviceScan.lookupProduct"), [variant?.product?.title, variant?.title].filter(Boolean).join(" · ")],
    [t("orderDevices.imei"), device.imei],
    [t("orderDevices.imei2"), device.imei2],
    [t("orderDevices.eid"), device.eid],
    [t("deviceScan.lookupState"), state(device.state)],
    [t("deviceScan.category"), t(`deviceScan.categories.${device.category}`)],
    [t("deviceScan.grade"), device.grade],
    [t("deviceScan.battery"), assessment.assessment?.battery === "unknown" ? t("deviceScan.buyback.unsure") : device.battery_health == null ? null : `${device.battery_health}%`],
    [t("deviceScan.lookupLocation"), query.data?.sources.find((entry) => entry.location_id === device.location_id)?.name || device.location_id],
    [t("deviceScan.lookupSource"), query.data?.sources.find((entry) => entry.id === device.source_id)?.name || device.source_id],
    [t("deviceScan.bin"), device.bin],
    [t("deviceScan.cost"), assessment.assessment?.priceAgreed === false ? t("deviceScan.buyback.pendingCost") : new Intl.NumberFormat(i18n.language.startsWith("zh") ? "zh-CN" : "en-GB", { style: "currency", currency: device.currency_code }).format(device.cost_cents / 100)],
    [t("deviceScan.lookupCreated"), date(device.created_at)],
    [t("deviceScan.lookupUpdated"), date(device.updated_at)],
    [t("deviceScan.lookupDeviceId"), device.id],
    [t("deviceScan.intakeNotes"), assessment.notes],
  ] : []
  return <FocusModal open onOpenChange={(open) => { if (!open) onClose() }}>
    <FocusModal.Content aria-describedby={undefined}>
      <FocusModal.Header><FocusModal.Title>{t("deviceScan.lookupDetails")}</FocusModal.Title>
        {device && query.data?.access.role === "admin" && <Button size="small" variant="secondary" onClick={() => setEditing(true)}>{t("deviceScan.editDevice")}</Button>}
        <FocusModal.Close asChild><Button size="small" variant="secondary">{t("deviceScan.lookupClose")}</Button></FocusModal.Close>
      </FocusModal.Header>
      <FocusModal.Body className="overflow-y-auto px-6 py-4">
        <div className="mx-auto w-full max-w-5xl space-y-6" data-testid="device-details">
          {query.isPending ? <Text role="status">{t("deviceScan.loading")}</Text> : query.isError ?
            <Button size="small" variant="secondary" onClick={() => query.refetch()}>{t("orderDevices.retry")}</Button> : !device ?
              <Text>{t("deviceScan.deviceNotFound")}</Text> : <>
                <Heading level="h2">{device.serial_number || device.id}</Heading>
                {!!query.data?.events.length && <DeviceProcessingHistory events={query.data.events} />}
                {assessment.assessment && <RecycledAssessmentSummary value={assessment.assessment} touchId={variant?.product?.title.includes("SE")} />}
                <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(min(100%, 260px), 1fr))", gap: 16 }}>
                  {fields.map(([label, value]) => <div key={label} className="min-w-0 space-y-1">
                    <Text size="small" className="text-ui-fg-subtle">{label}</Text>
                    <Text size="small" className="whitespace-pre-wrap break-all">{value || t("orderDevices.notRecorded")}</Text>
                  </div>)}
                </div>
                {!!device.photos?.length && <section className="space-y-3 border-t pt-4"><Heading level="h2">{t("deviceScan.photos")}</Heading><div className="flex flex-wrap gap-3">{device.photos.map((photo, index) => <a key={photo.id} href={photo.url} target="_blank" rel="noreferrer"><img src={photo.url} alt={`${t("deviceScan.photos")} ${index + 1}`} style={{ width: 160, height: 160, objectFit: "cover", borderRadius: 8 }} /></a>)}</div></section>}
                <section className="space-y-3 border-t pt-4">
                  <Heading level="h2">{t("deviceScan.lookupOrders")}</Heading>
                  {!query.data?.assignments.length && <Text size="small" className="text-ui-fg-subtle">{t("deviceScan.lookupNoOrders")}</Text>}
                  {query.data?.assignments.map((assignment) => <div key={assignment.id} className="flex flex-wrap gap-3 rounded-lg border p-3">
                    <a className="break-all text-sm text-ui-fg-interactive underline" href={`/app/orders/${assignment.order_id}`}>{assignment.order_id}</a>
                    <Text size="small">{state(assignment.status)}</Text>
                    {assignment.tracking_number && <Text size="small">{t("deviceScan.trackingPrefix")}{assignment.tracking_number}</Text>}
                  </div>)}
                </section>
                <section className="space-y-3 border-t pt-4">
                  <Heading level="h2">{t("deviceScan.history")}</Heading>
                  {!query.data?.events.length && <Text size="small" className="text-ui-fg-subtle">{t("deviceScan.lookupNoEvents")}</Text>}
                  {query.data?.events.map((event) => <details key={event.id} className="rounded-lg border p-3 text-sm">
                    <summary className="cursor-pointer"><span>{date(event.created_at)} · {t(`deviceScan.actions.${event.action}`, { defaultValue: event.action })}</span><span className="ml-3 break-all">{t("deviceScan.actorPrefix")}{event.actor_name || event.actor_id}{event.data?.mobile_intake_nonce ? ` · ${t("deviceScan.mobileIntakeActor")}` : ""}</span></summary>
                    <Text size="small" className="mt-2 break-all">{t("deviceScan.actorPrefix")}{event.actor_id}</Text>
                    <pre className="mt-2 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(event.data, null, 2)}</pre>
                  </details>)}
                </section>
              </>}
        </div>
      </FocusModal.Body>
      {editing && device && query.data && <DeviceEditDrawer device={device} sources={query.data.sources} variants={query.data.variants} onClose={() => setEditing(false)} />}
    </FocusModal.Content>
  </FocusModal>
}
