// Source: modules/home/components/blog-section and app/.../blog/[slug]/page.tsx.
// Native image and locale-explicit links replace Next-specific components.
import Image from '../products/product-image'
import Link from '../products/localized-link'
import {ArrowRight,ArrowLeft} from '../products/icons'
import {getHomeCopy} from './i18n'
import styles from './styles.module.css'

export default function Journal({segment,slug}:{segment:string;slug?:string}){
 const copy=getHomeCopy(segment.endsWith('-zh')?'zh-CN':'en')
 if(slug){
  const post=copy.blogPosts.find(post=>post.slug===slug)
  if(!post)return null
  return <div className={styles.article}>
   <Link segment={segment} className={styles.back} href="/blog"><ArrowLeft width={18} height={18} aria-hidden="true"/>{copy.blogBack}</Link>
   <p className={styles.category}>{post.category}</p><h1>{post.title}</h1><p>{post.description}</p>
   <div className={styles.articleImage}><Image src={post.image} alt="" fill priority sizes="(max-width: 799px) 90vw, 752px"/></div>
   {post.sections.map(section=><section key={section.title}><h2>{section.title}</h2><p>{section.text}</p></section>)}
   <Link segment={segment} href="/store" className={styles.back}>{copy.blogShop}<ArrowRight width={18} height={18} aria-hidden="true"/></Link>
  </div>
 }
 return <div className={styles.journal}><section className={styles.section} aria-labelledby="blog-title" data-testid="home-blog">
  <header className={styles.heading}><div><h1 id="blog-title">{copy.blogTitle}</h1><p>{copy.blogDescription}</p></div></header>
  <div className={styles.cards}>{copy.blogPosts.map(post=><article key={post.slug} className={styles.story}>
   <Link segment={segment} href={`/blog/${post.slug}`} className={styles.storyLink}>
    <div className={styles.storyImage}><Image src={post.image} alt="" fill sizes="(max-width: 699px) 90vw, 380px"/></div>
    <div className={styles.storyCopy}><p className={styles.category}>{post.category}</p><h3>{post.title}</h3><p>{post.description}</p><span className={styles.read}>{copy.blogRead}<ArrowRight width={18} height={18} aria-hidden="true"/></span></div>
   </Link>
  </article>)}</div>
 </section></div>
}
