import type { Metadata } from "next"
import { getLocale } from "@lib/data/locale-actions"
import { getHomeCopy } from "@modules/home/i18n"
import BlogSection from "@modules/home/components/blog-section"
import styles from "@modules/home/components/home-sections.module.css"

export async function generateMetadata(): Promise<Metadata> {
  const copy = getHomeCopy(await getLocale())
  return { title: copy.blogTitle, description: copy.blogDescription }
}

export default async function Blog() {
  return <main className={styles.journal}><BlogSection copy={getHomeCopy(await getLocale())} listing /></main>
}
