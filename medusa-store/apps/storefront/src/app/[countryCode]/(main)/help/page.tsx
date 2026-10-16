import { Metadata } from "next"
import { getLocale } from "@lib/data/locale-actions"
import { getLayoutCopy } from "@modules/layout/i18n"
import InformationPage from "@modules/layout/templates/information-page"

export async function generateMetadata(): Promise<Metadata> {
  const copy = getLayoutCopy(await getLocale())
  return { title: copy.help, description: copy.helpDescription }
}

export default function Page() {
  return <InformationPage kind="help" />
}