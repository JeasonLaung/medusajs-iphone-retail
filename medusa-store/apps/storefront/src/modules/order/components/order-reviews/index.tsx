"use client"

import { useEffect, useState } from "react"
import { HttpTypes } from "@medusajs/types"
import { useAccountCopy } from "@modules/account/use-account-copy"
import { getOrderReviewContext } from "@lib/data/product-reviews"
import type { OrderReviewContext } from "types/product-reviews"
import ReviewForm from "@modules/products/components/product-reviews/review-form"
import { getReviewCopy } from "@modules/products/components/product-reviews/i18n"
import { Button } from "@modules/account/components/ui"

export default function OrderReviews({
  order,
}: {
  order: HttpTypes.StoreOrder
}) {
  const { locale } = useAccountCopy()
  const copy = getReviewCopy(locale)
  const [context, setContext] = useState<OrderReviewContext | null>(null)
  const [loading, setLoading] = useState(true)
  const [retry, setRetry] = useState(0)
  const [activeProduct, setActiveProduct] = useState<string | null>(null)
  const [submitted, setSubmitted] = useState<string[]>([])
  const products = Array.from(
    new Map(
      (order.items || []).flatMap((item) => {
        const id = item.product_id || item.product?.id
        return id ? [[id, item.product_title || item.title] as const] : []
      })
    ).entries()
  )

  useEffect(() => {
    let active = true
    setLoading(true)
    getOrderReviewContext(order.id).then((result) => {
      if (active) {
        setContext(result)
        setLoading(false)
      }
    })
    return () => {
      active = false
    }
  }, [order.id, retry])

  if (!products.length) return null
  return (
    <section
      id="order-reviews"
      className="mt-6 scroll-mt-32 rounded-xl border border-ui-border-base p-6"
      data-testid="order-reviews"
    >
      <h2 className="text-lg font-semibold">{copy.orderHeading}</h2>
      <p className="mt-1 text-sm text-ui-fg-subtle">{copy.orderIntro}</p>
      {loading ? (
        <p role="status" className="mt-4 text-sm">
          {copy.loadingOrders}
        </p>
      ) : !context ? (
        <div role="alert" className="mt-4 text-sm">
          {copy.purchasesError}{" "}
          <Button variant="secondary" onClick={() => setRetry(retry + 1)}>
            {copy.retry}
          </Button>
        </div>
      ) : (
        <>
          {!context.eligible && (
            <p className="mt-4 text-sm text-ui-fg-subtle">
              {copy.orderIneligible}
            </p>
          )}
          <div className="mt-4 space-y-4">
            {products.map(([productId, title]) => {
              const review = context.reviews.find(
                (review) => review.product_id === productId
              )
              const status = submitted.includes(productId)
                ? "pending"
                : review?.status
              return (
                <div
                  key={productId}
                  className="border-t border-ui-border-base pt-4"
                >
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <h3 className="font-medium">{title}</h3>
                    {status ? (
                      <p role="status" className="text-sm text-ui-fg-subtle">
                        {copy[status]}
                        {review && !review.verified_purchase
                          ? ` · ${copy.openReview}`
                          : ""}
                      </p>
                    ) : (
                      <Button
                        variant="secondary"
                        disabled={!context.eligible}
                        aria-expanded={activeProduct === productId}
                        onClick={() =>
                          setActiveProduct(
                            activeProduct === productId ? null : productId
                          )
                        }
                      >
                        {copy.write}
                      </Button>
                    )}
                  </div>
                  {activeProduct === productId && !status && (
                    <div className="mt-4">
                      <ReviewForm
                        antDesign
                        productId={productId}
                        locale={locale}
                        orderId={order.id}
                        onCancel={() => setActiveProduct(null)}
                        onSuccess={() => {
                          setSubmitted((current) => [...current, productId])
                          setActiveProduct(null)
                        }}
                      />
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </>
      )}
    </section>
  )
}
