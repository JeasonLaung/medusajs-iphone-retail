import { withSEO } from "@lib/data/seo"
import { Metadata } from "next"
import { getLocale } from "@lib/data/locale-actions"
import { getSellCopy } from "@modules/sell/i18n"
import SellLanding from "@modules/sell/components/sell-landing"

async function defaultPageMetadata(): Promise<Metadata> {
  const copy = getSellCopy(await getLocale())
  return { title: copy.title, description: copy.intro }
}

export default async function Page() {
  return <SellLanding locale={await getLocale()} />
}

export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
