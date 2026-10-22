import { Metadata } from "next"
import { notFound } from "next/navigation"
import { getLocale } from "@lib/data/locale-actions"
import { getSellModels } from "@lib/data/sell-models"
import { getSellCopy } from "@modules/sell/i18n"
import SellFlow from "@modules/sell/components/sell-flow"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import styles from "@modules/sell/flow.module.css"

export async function generateMetadata(): Promise<Metadata> {
  const copy = getSellCopy(await getLocale())
  return { title: copy.title, description: copy.intro, robots: { index: false } }
}

export default async function Page({ params }: { params: Promise<{ countryCode: string; steps: string[] }> }) {
  const { countryCode, steps } = await params
  const locale = await getLocale()
  const copy = getSellCopy(locale)
  let models: Awaited<ReturnType<typeof getSellModels>> = []
  try { models = await getSellModels() } catch {}
  if (!models.length) return <div className={styles.page}><div className={styles.container}>
    <h1>{copy.modelTitle}</h1><p role="status">{copy.unavailable}</p>
    <a href={`/${countryCode}/sell/iphone/model`} className={styles.primary}>{copy.retry}</a>
    <LocalizedClientLink href="/sell">{copy.back}</LocalizedClientLink>
  </div></div>
  const valid = steps.length === 1 && steps[0] === "model" ||
    steps.length === 2 && models.some((m) => m.handle === steps[0]) && steps[1] === "memory" ||
    steps.length === 3 && models.some((m) => m.handle === steps[0] && m.capacities.some((c) => c.toLowerCase() === steps[1])) &&
      ["battery", "functionality", "screen-faults", "screen-glass", "screen", "frame", "back", "summary"].includes(steps[2])
  if (!valid) notFound()
  return <SellFlow models={models} steps={steps} locale={locale} />
}
