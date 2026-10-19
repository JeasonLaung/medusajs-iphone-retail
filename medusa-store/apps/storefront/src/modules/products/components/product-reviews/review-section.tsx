"use client"

import { useState, useTransition } from "react"
import { Star, StarSolid } from "@medusajs/icons"
import { listProductReviews } from "@lib/data/product-reviews"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import type { ProductReviewPage } from "types/product-reviews"
import { getReviewCopy } from "./i18n"
import ReviewForm from "./review-form"
import styles from "./styles.module.css"

const Stars = ({ rating }: { rating: number }) => (
  <span className={styles.starIcons} aria-hidden="true">
    {[1, 2, 3, 4, 5].map((value) =>
      value <= Math.round(rating) ? (
        <StarSolid key={value} />
      ) : (
        <Star key={value} />
      )
    )}
  </span>
)

export default function ReviewSection({
  productId,
  locale,
  initialPage,
  signedIn,
}: {
  productId: string
  locale: string
  initialPage: ProductReviewPage | null
  signedIn: boolean
}) {
  const copy = getReviewCopy(locale)
  const chinese = locale.startsWith("zh") || locale.endsWith("-zh")
  const intlLocale = chinese ? "zh-CN" : "en-GB"
  const [page, setPage] = useState(initialPage)
  const [loadError, setLoadError] = useState(!initialPage)
  const [formOpen, setFormOpen] = useState(false)
  const [success, setSuccess] = useState(false)
  const [loading, startLoading] = useTransition()
  const starsLabel = (value: number) =>
    copy.stars.replace(
      "{rating}",
      new Intl.NumberFormat(intlLocale, { maximumFractionDigits: 1 }).format(
        value
      )
    )

  function load(offset: number) {
    startLoading(async () => {
      try {
        const result = await listProductReviews(productId, offset)
        if (result) {
          setPage(result)
          setLoadError(false)
        } else setLoadError(true)
      } catch {
        setLoadError(true)
      }
    })
  }

  return (
    <section
      id="product-reviews"
      aria-labelledby="reviews-heading"
      className={styles.section}
      data-testid="product-reviews"
    >
      <header className={styles.header}>
        <div>
          <h2 id="reviews-heading">{copy.heading}</h2>
          <p>{copy.intro}</p>
        </div>
        {signedIn ? (
          <button
            type="button"
            className={styles.button}
            disabled={success}
            aria-expanded={formOpen}
            aria-controls="review-form"
            onClick={() => setFormOpen(!formOpen)}
          >
            {copy.write}
          </button>
        ) : (
          <LocalizedClientLink className={styles.button} href="/account">
            {copy.signIn}
          </LocalizedClientLink>
        )}
      </header>
      {success && (
        <p className={styles.notice} role="status">
          {copy.success}
        </p>
      )}
      {formOpen && !success && (
        <div id="review-form">
          <ReviewForm
            productId={productId}
            locale={locale}
            onSuccess={() => {
              setSuccess(true)
              setFormOpen(false)
            }}
            onCancel={() => setFormOpen(false)}
          />
        </div>
      )}
      {loadError && (
        <div className={styles.notice} role="alert">
          {copy.loadError}{" "}
          <button
            disabled={loading}
            onClick={() => load(page?.offset || 0)}
            className={styles.link}
          >
            {copy.retry}
          </button>
        </div>
      )}
      <div aria-busy={loading}>
        {page && page.count > 0 && (
          <div className={styles.layout}>
            <aside className={styles.summary} aria-label={copy.heading}>
              <strong>
                {new Intl.NumberFormat(intlLocale, {
                  minimumFractionDigits: 1,
                  maximumFractionDigits: 1,
                }).format(page.average)}
              </strong>
              <span className={styles.muted}>{copy.outOf}</span>
              <p className={styles.stars} aria-label={starsLabel(page.average)}>
                <Stars rating={page.average} />
              </p>
              <p>
                {copy.count.replace(
                  "{count}",
                  page.count.toLocaleString(intlLocale)
                )}
              </p>
              <div className={styles.distribution}>
                {[...page.distribution].reverse().map((row) => (
                  <div
                    key={row.rating}
                    aria-label={`${starsLabel(row.rating)}: ${row.count}`}
                  >
                    <span className={styles.ratingLabel} aria-hidden="true">
                      {row.rating}
                      <StarSolid />
                    </span>
                    <div className={styles.track} aria-hidden="true">
                      <span
                        style={{ width: `${(row.count / page.count) * 100}%` }}
                      />
                    </div>
                    <span aria-hidden="true">
                      {row.count.toLocaleString(intlLocale)}
                    </span>
                  </div>
                ))}
              </div>
            </aside>
            <div className={styles.list}>
              <p className={styles.muted}>{copy.newest}</p>
              {page.reviews.map((review) => (
                <article key={review.id} className={styles.review}>
                  <div className={styles.reviewMeta}>
                    <strong>{review.author || copy.customer}</strong>
                    <time dateTime={review.created_at}>
                      {new Date(review.created_at).toLocaleDateString(
                        intlLocale,
                        {
                          year: "numeric",
                          month: "short",
                          day: "numeric",
                          timeZone: "UTC",
                        }
                      )}
                    </time>
                  </div>
                  {review.verified_purchase && (
                    <span className={styles.verified}>{copy.verified}</span>
                  )}
                  <p
                    className={styles.stars}
                    aria-label={starsLabel(review.rating)}
                  >
                    <Stars rating={review.rating} />
                  </p>
                  <h3>{review.title}</h3>
                  <p className={styles.content}>{review.content}</p>
                </article>
              ))}
              {page.count > page.limit && (
                <nav className={styles.pagination} aria-label={copy.heading}>
                  <button
                    className={styles.link}
                    disabled={loading || page.offset === 0}
                    onClick={() => load(Math.max(0, page.offset - page.limit))}
                  >
                    {copy.previous}
                  </button>
                  <span>
                    {copy.page
                      .replace(
                        "{page}",
                        String(Math.floor(page.offset / page.limit) + 1)
                      )
                      .replace(
                        "{pages}",
                        String(Math.ceil(page.count / page.limit))
                      )}
                  </span>
                  <button
                    className={styles.link}
                    disabled={loading || page.offset + page.limit >= page.count}
                    onClick={() => load(page.offset + page.limit)}
                  >
                    {copy.next}
                  </button>
                </nav>
              )}
            </div>
          </div>
        )}
        {page?.count === 0 && (
          <div className={styles.empty}>
            <span className={styles.emptyStars}>
              <Stars rating={0} />
            </span>
            <h3>{copy.empty}</h3>
            <p>{copy.emptyDescription}</p>
            {!signedIn && <p className={styles.muted}>{copy.signInNote}</p>}
          </div>
        )}
      </div>
      <p role="status" className={styles.loading}>
        {loading ? copy.loading : ""}
      </p>
    </section>
  )
}
