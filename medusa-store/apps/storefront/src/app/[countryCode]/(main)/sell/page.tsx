import { Metadata } from "next"
import { getLocale } from "@lib/data/locale-actions"
import { getLayoutCopy } from "@modules/layout/i18n"
import InformationPage from "@modules/layout/templates/information-page"

export async function generateMetadata(): Promise<Metadata> {
  const copy = getLayoutCopy(await getLocale())
  return { title: copy.sell, description: copy.sellDescription }
}

export default function Page() {
  return <InformationPage kind="sell" />
}