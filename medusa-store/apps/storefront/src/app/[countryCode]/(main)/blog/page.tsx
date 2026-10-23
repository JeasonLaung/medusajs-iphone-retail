import { withSEO } from "@lib/data/seo"
import type { Metadata } from "next"
import { getLocale } from "@lib/data/locale-actions"
import { getHomeCopy } from "@modules/home/i18n"
import BlogSection from "@modules/home/components/blog-section"
import styles from "@modules/home/components/home-sections.module.css"

async function defaultPageMetadata(): Promise<Metadata> {
  const copy = getHomeCopy(await getLocale())
  return { title: copy.blogTitle, description: copy.blogDescription }
}

export default async function Blog() {
  return <main className={styles.journal}><BlogSection copy={getHomeCopy(await getLocale())} listing /></main>
}

export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
