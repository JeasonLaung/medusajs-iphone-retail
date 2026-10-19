import Image from "next/image"
import { ArrowRight } from "@medusajs/icons"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import type { HomeCopy } from "@modules/home/i18n"
import styles from "../home-sections.module.css"

export default function BlogSection({ copy, listing = false }: { copy: HomeCopy; listing?: boolean }) {
  const Heading = listing ? "h1" : "h2"
  return <section className={styles.section} aria-labelledby="blog-title" data-testid="home-blog">
    <header className={styles.heading}><div><Heading id="blog-title">{copy.blogTitle}</Heading><p>{copy.blogDescription}</p></div>{!listing && <LocalizedClientLink href="/blog">{copy.blogAll}<ArrowRight width={18} height={18} aria-hidden="true" /></LocalizedClientLink>}</header>
    <div className={styles.cards}>{copy.blogPosts.map((post) => <article key={post.slug} className={styles.story}>
      <LocalizedClientLink href={`/blog/${post.slug}`} className={styles.storyLink}>
        <div className={styles.storyImage}><Image src={post.image} alt="" fill sizes="(max-width: 699px) 90vw, 380px" /></div>
        <div className={styles.storyCopy}><p className={styles.category}>{post.category}</p><h3>{post.title}</h3><p>{post.description}</p><span className={styles.read}>{copy.blogRead}<ArrowRight width={18} height={18} aria-hidden="true" /></span></div>
      </LocalizedClientLink>
    </article>)}</div>
  </section>
}
