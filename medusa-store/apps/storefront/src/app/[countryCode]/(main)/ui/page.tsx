import type { Metadata } from "next"
import { getLocale } from "@lib/data/locale-actions"
import UiShowcase from "@modules/ui-showcase"
import { getUiCopy } from "@modules/ui-showcase/i18n"

export async function generateMetadata(): Promise<Metadata> {
  const copy = getUiCopy(await getLocale())
  return { title: copy.title, robots: { index: false, follow: false } }
}

export default async function Page() {
  return <UiShowcase locale={await getLocale()} />
}
