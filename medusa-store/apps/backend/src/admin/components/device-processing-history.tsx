import { Text } from "@medusajs/ui"
import { useDeviceTranslations } from "../lib/use-device-translations"
import type { DevicePhoto } from "./device-photos-input"

export type ProcessingEvent = {
  id: string
  created_at: string
  action: string
  actor_id: string
  actor_name?: string
  data: { notes?: string; photos?: DevicePhoto[]; before?: { state?: string }; after?: { state?: string }; [key: string]: unknown }
}

export default function DeviceProcessingHistory({ events }: { events: ProcessingEvent[] }) {
  const { t, i18n } = useDeviceTranslations()
  const history = [...events].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
  const state = (value?: string) => value ? t(`deviceScan.${({ repair_ready: "repairReady", supplier_returned: "supplierReturned" } as Record<string, string>)[value] || value}`) : t("orderDevices.notRecorded")
  return <section className="space-y-3" aria-label={t("deviceScan.returnProcessing.history")}>
    <Text size="small" weight="plus">{t("deviceScan.returnProcessing.history")}</Text>
    {!history.length && <Text size="small" className="text-ui-fg-subtle">{t("deviceScan.returnProcessing.noHistory")}</Text>}
    {history.map((event) => <div key={event.id} className="space-y-1 rounded-md bg-ui-bg-subtle p-3">
      <Text size="small" className="text-ui-fg-subtle">{new Date(event.created_at).toLocaleString(i18n.language.startsWith("zh") ? "zh-CN" : "en-GB")}</Text>
      <Text size="small" className="break-words">{t("deviceScan.actorPrefix")}{event.actor_name || event.actor_id}</Text>
      <Text size="small" weight="plus">{t(`deviceScan.actions.${event.action}`, { defaultValue: event.action })}</Text>
      {(event.data.before?.state || event.data.after?.state) && <Text size="small">{t("deviceScan.returnProcessing.transition", { before: state(event.data.before?.state), after: state(event.data.after?.state) })}</Text>}
      {event.data.notes && <Text size="small" className="whitespace-pre-wrap break-words">{event.data.notes}</Text>}
      {!!event.data.photos?.length && <div className="flex flex-wrap gap-2">{event.data.photos.map((photo, index) => <a key={photo.id} href={photo.url} target="_blank" rel="noreferrer"><img src={photo.url} alt={t("deviceScan.returnProcessing.photoAlt", { number: index + 1 })} className="h-24 w-24 rounded-md object-cover" /></a>)}</div>}
    </div>)}
  </section>
}
