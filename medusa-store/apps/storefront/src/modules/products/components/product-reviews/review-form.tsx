"use client"

import { Input as AntInput } from "antd"
import { Button as AntButton } from "@modules/account/components/ui"
import { useEffect, useId, useState, useTransition } from "react"
import { RatingInput } from "@modules/common/components/rating"
import {
  getReviewPurchases,
  submitProductReview,
} from "@lib/data/product-reviews"
import type { ReviewPurchases } from "types/product-reviews"
import { getReviewCopy } from "./i18n"
import styles from "./styles.module.css"

export default function ReviewForm({
  antDesign = false,
  productId,
  locale,
  orderId,
  onSuccess,
  onCancel,
}: {
  antDesign?: boolean
  productId: string
  locale: string
  orderId?: string
  onSuccess: () => void
  onCancel: () => void
}) {
  const TitleInput = antDesign ? AntInput : "input"
  const ContentInput = antDesign ? AntInput.TextArea : "textarea"
  const ActionButton = antDesign ? AntButton : "button"
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
      className={
        antDesign ? "my-4 p-4 bg-ui-bg-subtle rounded-md" : styles.form
      }
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
      <fieldset
        disabled={submitting}
        className={antDesign ? "min-w-0 flex flex-col gap-3" : styles.fields}
      >
        <legend>{copy.rating}</legend>
        <div className={styles.ratingOptions}>
          <RatingInput
            value={rating}
            onChange={setRating}
            disabled={submitting}
            label={copy.rating}
            starLabels={[1, 2, 3, 4, 5].map((value) =>
              copy.stars.replace("{rating}", String(value))
            )}
          />
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
                <ActionButton
                  type="button"
                  className={antDesign ? undefined : styles.link}
                  onClick={() => setRetry(retry + 1)}
                >
                  {copy.retry}
                </ActionButton>
              </p>
            )}
            {purchases &&
              purchases.offset + purchases.limit < purchases.count && (
                <ActionButton
                  type="button"
                  disabled={ordersLoading}
                  className={antDesign ? undefined : styles.link}
                  onClick={moreOrders}
                >
                  {copy.moreOrders}
                </ActionButton>
              )}
          </>
        )}
        <label className={styles.label} htmlFor={`${id}-title`}>
          {copy.title}
        </label>
        <TitleInput
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
        <ContentInput
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
          <ActionButton
            className={antDesign ? undefined : styles.button}
            type="submit"
            disabled={ordersLoading}
          >
            {submitting ? copy.submitting : copy.submit}
          </ActionButton>
          <ActionButton
            className={antDesign ? undefined : styles.link}
            type="button"
            onClick={onCancel}
          >
            {copy.cancel}
          </ActionButton>
        </div>
      </fieldset>
    </form>
  )
}
