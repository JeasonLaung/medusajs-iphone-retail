import { withSEO } from "@lib/data/seo"
import { Metadata } from "next"
import { getLocale } from "@lib/data/locale-actions"
import { getLayoutCopy } from "@modules/layout/i18n"
import InformationPage from "@modules/layout/templates/information-page"
import { getTrustpilotSettings } from "@lib/data/trustpilot"
import TrustpilotReviews from "@modules/common/components/trustpilot/reviews"

async function defaultPageMetadata(): Promise<Metadata> {
  const copy = getLayoutCopy(await getLocale())
  return { title: copy.reviews, description: copy.reviewsDescription }
}

export default async function Page() {
  const settings = await getTrustpilotSettings()
  if (settings) return <main className="content-container py-12"><TrustpilotReviews settings={settings} locale={await getLocale()} /></main>
  return <InformationPage kind="reviews" />
}
export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
