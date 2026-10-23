import { Heading, Text } from "@medusajs/ui"
import { useDeviceTranslations } from "../lib/use-device-translations"
import { faultIds, glassIds, gradeIds, skipScreen, type RecycledAssessment } from "../lib/recycled-assessment"

export default function RecycledAssessmentSummary({ value, touchId = false }: { value: RecycledAssessment; touchId?: boolean }) {
  const { t } = useDeviceTranslations()
  const copy = (key: string) => t(`deviceScan.buyback.${key}`)
  const rows = [
    [copy("titles.2"), value.battery === "unknown" ? copy("unsure") : `${value.battery}%`],
    ...value.checks.map((answer, index) => [copy(index === 2 && touchId ? "touchId.2" : `checks.${index}.2`), copy(answer ? "yes" : "no")]),
    [copy("titles.4"), value.faults.map((id) => copy(id === "none" ? "allWorks" : `faults.${faultIds.indexOf(id)}`)).join(", ")],
    [copy("titles.5"), value.glass.map((id) => copy(id === "none" ? "none" : `glass.${glassIds.indexOf(id)}.0`)).join(", ")],
    ...(["screen", "frame", "back"] as const).filter((field) => !(field === "screen" && skipScreen(value)) && !(field === "back" && value.frame === "damaged")).map((field) => [copy(`titles.${field === "screen" ? 6 : field === "frame" ? 7 : 8}`), copy(`grades.${gradeIds.indexOf(value[field])}`)]),
  ]
  return <section className="space-y-4">
    <Heading level="h2">{copy("report")}</Heading>
    <dl className="grid gap-4 sm:grid-cols-2">
      {rows.map(([label, answer]) => <div key={label} className="min-w-0 space-y-1"><dt><Text size="small" className="text-ui-fg-subtle">{label}</Text></dt><dd><Text size="small">{answer}</Text></dd></div>)}
    </dl>
  </section>
}
