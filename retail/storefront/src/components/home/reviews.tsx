import { ArrowRight } from "../products/icons"
import type { HomeReviewsData } from "../../lib/home"
import LocalizedClientLink from "./localized-link"
import type { HomeCopy } from "../journal/i18n"
import Rating from "./rating"
import styles from "./sections.module.css"

export default function HomeReviews({ copy, reviews, unavailable }: { copy: HomeCopy } & HomeReviewsData) {
  return <section className={styles.section} aria-labelledby="home-reviews-title" data-testid="home-reviews">
    <header className={styles.heading}><div><h2 id="home-reviews-title">{copy.reviewsTitle}</h2><p>{copy.reviewsDescription}</p></div><LocalizedClientLink href="/reviews">{copy.reviewsLink}<ArrowRight width={18} height={18} aria-hidden="true" /></LocalizedClientLink></header>
    {reviews.length ? <div className={styles.cards}>{reviews.map((review) => <article className={styles.review} key={review.id}>
      <div className={styles.rating}><Rating value={review.rating} label={copy.reviewRating.replace("{rating}", String(review.rating))} size={18} /></div>
      <h3>{review.title}</h3><p className={styles.reviewText}>{review.content}</p><footer><strong>{review.author}</strong><LocalizedClientLink href={`/products/${review.product.handle}`}>{review.product.title}</LocalizedClientLink></footer>
    </article>)}</div> : <div className={styles.empty}>
      <svg viewBox="0 0 48 48" width="44" height="44" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><path d="M38 31a5 5 0 0 1-5 5H19l-9 6V13a5 5 0 0 1 5-5h18a5 5 0 0 1 5 5Z" /><path d="M17 18h14M17 25h10" /></svg>
      <h3>{copy.reviewsEmptyTitle}</h3><p>{unavailable ? copy.reviewsUnavailable : copy.reviewsEmpty}</p><LocalizedClientLink href="/store">{copy.reviewsShop}<ArrowRight width={18} height={18} aria-hidden="true" /></LocalizedClientLink>
    </div>}
  </section>
}
