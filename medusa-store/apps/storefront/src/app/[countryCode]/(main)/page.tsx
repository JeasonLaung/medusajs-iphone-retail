import type { Metadata } from "next"
import { getLocale } from "@lib/data/locale-actions"
import SeriesGallery from "@modules/home/components/series-gallery"
import { getHomeCopy } from "@modules/home/i18n"

export async function generateMetadata(): Promise<Metadata> {
  const copy = getHomeCopy(await getLocale())
  return { title: copy.metaTitle, description: copy.description }
}

export default async function Home() {
  return <SeriesGallery copy={getHomeCopy(await getLocale())} />
}
