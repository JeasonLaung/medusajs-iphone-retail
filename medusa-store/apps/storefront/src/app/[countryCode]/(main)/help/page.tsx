import { withSEO } from "@lib/data/seo"
import { Metadata } from "next"
import { getLocale } from "@lib/data/locale-actions"
import { getLayoutCopy } from "@modules/layout/i18n"
import InformationPage from "@modules/layout/templates/information-page"

async function defaultPageMetadata(): Promise<Metadata> {
  const copy = getLayoutCopy(await getLocale())
  return { title: copy.help, description: copy.helpDescription }
}

export default function Page() {
  return <InformationPage kind="help" />
}
export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
