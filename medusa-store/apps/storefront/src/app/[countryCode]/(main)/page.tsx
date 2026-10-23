import { withSEO } from "@lib/data/seo"
import type { Metadata } from "next"
import { getLocale } from "@lib/data/locale-actions"
import SeriesGallery from "@modules/home/components/series-gallery"
import { getHomeCopy } from "@modules/home/i18n"

async function defaultPageMetadata(): Promise<Metadata> {
  const copy = getHomeCopy(await getLocale())
  return { title: copy.metaTitle, description: copy.description }
}

export default async function Home() {
  return <SeriesGallery copy={getHomeCopy(await getLocale())} />
}

export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
