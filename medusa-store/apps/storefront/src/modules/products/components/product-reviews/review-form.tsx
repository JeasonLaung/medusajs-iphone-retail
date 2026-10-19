"use client"

import { useEffect, useId, useState, useTransition } from "react"
import { StarSolid } from "@medusajs/icons"
import {
  getReviewPurchases,
  submitProductReview,
} from "@lib/data/product-reviews"
import type { ReviewPurchases } from "types/product-reviews"
import { getReviewCopy } from "./i18n"
import styles from "./styles.module.css"

export default function ReviewForm({
  productId,
  locale,
  orderId,
  onSuccess,
  onCancel,
}: {
  productId: string
  locale: string
  orderId?: string
  onSuccess: () => void
  onCancel: () => void
}) {
  const copy = getReviewCopy(locale)
  const id = useId()
  const intlLocale =
    locale.startsWith("zh") || locale.endsWith("-zh") ? "zh-CN" : "en-GB"
  const [rating, setRating] = useState(5)
  const [selectedOrder, setSelectedOrder] = useState(orderId || "")
  const [purchases, setPurchases] = useState<ReviewPurchases | null>(null)
  const [ordersLoading, setOrdersLoading] = useState(!orderId)
  const [ordersError, setOrdersError] = useState(false)
  const [retry, setRetry] = useState(0)
  const [error, setError] = useState<
    "login" | "duplicate" | "order" | "submit" | ""
  >("")
  const [submitting, startSubmitting] = useTransition()

  useEffect(() => {
    if (orderId) return
    let active = true
    setOrdersLoading(true)
    getReviewPurchases(productId).then((result) => {
      if (!active) return
      setPurchases(result)
      setOrdersError(!result)
      setOrdersLoading(false)
    })
    return () => {
      active = false
    }
  }, [productId, orderId, retry])

  async function moreOrders() {
    if (!purchases) return
    setOrdersLoading(true)
    const result = await getReviewPurchases(
      productId,
      purchases.offset + purchases.limit
    )
    if (result)
      setPurchases({
        ...result,
        orders: [...purchases.orders, ...result.orders],
      })
    setOrdersError(!result)
    setOrdersLoading(false)
  }

  if (purchases?.review)
    return (
      <p className={styles.notice} role="status">
        {copy.duplicate} {copy[purchases.review.status]}
      </p>
    )

  return (
    <form
      className={styles.form}
      data-testid="review-form"
      onSubmit={(event) => {
        event.preventDefault()
        const data = new FormData(event.currentTarget)
        setError("")
        startSubmitting(async () => {
          try {
            const result = await submitProductReview({
              product_id: productId,
              ...(selectedOrder ? { order_id: selectedOrder } : {}),
              rating,
              title: String(data.get("title") || "").trim(),
              content: String(data.get("content") || "").trim(),
            })
            if (result.error) setError(result.error)
            else onSuccess()
          } catch {
            setError("submit")
          }
        })
      }}
    >
      <fieldset disabled={submitting} className={styles.fields}>
        <legend>{copy.rating}</legend>
        <div className={styles.ratingOptions}>
          {[1, 2, 3, 4, 5].map((value) => (
            <label key={value}>
              <input
                type="radio"
                name="rating"
                value={value}
                checked={rating === value}
                onChange={() => setRating(value)}
                aria-label={copy.stars.replace("{rating}", String(value))}
              />
              <span aria-hidden="true" data-active={value <= rating}>
                <StarSolid />
              </span>
            </label>
          ))}
        </div>
        {orderId ? (
          <p className={styles.muted}>{copy.verified}</p>
        ) : (
          <>
            <label className={styles.label} htmlFor={`${id}-order`}>
              {copy.linkOrder}
            </label>
            <select
              id={`${id}-order`}
              value={selectedOrder}
              onChange={(event) => setSelectedOrder(event.target.value)}
              disabled={ordersLoading}
            >
              <option value="">{copy.noOrder}</option>
              {purchases?.orders
                .filter((order) => order.eligible)
                .map((order) => (
                  <option key={order.id} value={order.id}>
                    {copy.orderOption
                      .replace("{number}", order.number)
                      .replace(
                        "{date}",
                        new Date(order.created_at).toLocaleDateString(
                          intlLocale,
                          { timeZone: "UTC" }
                        )
                      )}
                  </option>
                ))}
            </select>
            <p className={styles.muted}>{copy.orderNote}</p>
            {ordersLoading && (
              <p role="status" className={styles.muted}>
                {copy.loadingOrders}
              </p>
            )}
            {ordersError && (
              <p role="alert" className={styles.error}>
                {copy.purchasesError}{" "}
                <button
                  type="button"
                  className={styles.link}
                  onClick={() => setRetry(retry + 1)}
                >
                  {copy.retry}
                </button>
              </p>
            )}
            {purchases &&
              purchases.offset + purchases.limit < purchases.count && (
                <button
                  type="button"
                  disabled={ordersLoading}
                  className={styles.link}
                  onClick={moreOrders}
                >
                  {copy.moreOrders}
                </button>
              )}
          </>
        )}
        <label className={styles.label} htmlFor={`${id}-title`}>
          {copy.title}
        </label>
        <input
          id={`${id}-title`}
          name="title"
          required
          minLength={2}
          maxLength={120}
          placeholder={copy.titlePlaceholder}
        />
        <label className={styles.label} htmlFor={`${id}-content`}>
          {copy.content}
        </label>
        <textarea
          id={`${id}-content`}
          name="content"
          required
          minLength={10}
          maxLength={3000}
          rows={5}
          placeholder={copy.contentPlaceholder}
        />
        <p className={styles.muted}>{copy.moderation}</p>
        {error && (
          <p className={styles.error} role="alert">
            {error === "duplicate"
              ? copy.duplicate
              : error === "login"
              ? copy.loginError
              : error === "order"
              ? copy.orderError
              : copy.submitError}
          </p>
        )}
        <div className={styles.formActions}>
          <button
            className={styles.button}
            type="submit"
            disabled={ordersLoading}
          >
            {submitting ? copy.submitting : copy.submit}
          </button>
          <button className={styles.link} type="button" onClick={onCancel}>
            {copy.cancel}
          </button>
        </div>
      </fieldset>
    </form>
  )
}
