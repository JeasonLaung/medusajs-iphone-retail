import type { Metadata } from "next"
import Image from "next/image"
import { notFound } from "next/navigation"
import { ArrowLeft, ArrowRight } from "@medusajs/icons"
import { getLocale } from "@lib/data/locale-actions"
import { getHomeCopy } from "@modules/home/i18n"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import styles from "@modules/home/components/home-sections.module.css"

type Props = { params: Promise<{ slug: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params
  const post = getHomeCopy(await getLocale()).blogPosts.find((post) => post.slug === slug)
  if (!post) notFound()
  return { title: post.title, description: post.description }
}

export default async function Article({ params }: Props) {
  const { slug } = await params
  const copy = getHomeCopy(await getLocale())
  const post = copy.blogPosts.find((post) => post.slug === slug)
  if (!post) notFound()
  return <main className={styles.article}>
    <LocalizedClientLink className={styles.back} href="/blog"><ArrowLeft width={18} height={18} aria-hidden="true" />{copy.blogBack}</LocalizedClientLink>
    <p className={styles.category}>{post.category}</p><h1>{post.title}</h1><p>{post.description}</p>
    <div className={styles.articleImage}><Image src={post.image} alt="" fill priority sizes="(max-width: 799px) 90vw, 752px" /></div>
    {post.sections.map((section) => <section key={section.title}><h2>{section.title}</h2><p>{section.text}</p></section>)}
    <LocalizedClientLink href="/store" className={styles.back}>{copy.blogShop}<ArrowRight width={18} height={18} aria-hidden="true" /></LocalizedClientLink>
  </main>
}
