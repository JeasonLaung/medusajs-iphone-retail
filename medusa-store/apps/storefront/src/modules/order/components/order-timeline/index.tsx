"use client"

import { useState } from "react"
import { Steps } from "antd"
import { retrieveOrderTimeline } from "@lib/data/orders"
import { convertToLocale } from "@lib/util/money"
import { useAccountCopy } from "@modules/account/use-account-copy"
import type { AccountKey } from "@modules/account/i18n"
import { Button } from "@modules/common/components/ui"
import type { OrderTimeline, OrderTimelineKind } from "types/order-timeline"
import styles from "./styles.module.css"

const labels: Record<OrderTimelineKind, AccountKey> = {
  placed: "timelinePlaced", paid: "timelinePaid", refund: "timelineRefund",
  allocated: "timelineAllocated", released: "timelineReleased", reallocated: "timelineReallocated",
  packed: "timelinePacked", shipped: "timelineShipped", delivered: "timelineDelivered",
  fulfillment_canceled: "timelineFulfillmentCanceled", canceled: "timelineCanceled",
}

export default function OrderTimelineSteps({ orderId, initialTimeline }: {
  orderId: string
  initialTimeline: OrderTimeline | null
}) {
  const { t, locale } = useAccountCopy()
  const [timeline, setTimeline] = useState(initialTimeline)
  const [loading, setLoading] = useState(false)

  async function retry() {
    setLoading(true)
    try {
      setTimeline(await retrieveOrderTimeline(orderId))
    } finally {
      setLoading(false)
    }
  }

  return (
    <section className={`${styles.timeline} min-w-0`} aria-labelledby={`order-timeline-${orderId}`} data-testid="order-timeline">
      <h2 id={`order-timeline-${orderId}`} className="text-lg font-semibold">{t("timelineTitle")}</h2>
      {!timeline ? (
        <div className="mt-4 flex flex-wrap items-center gap-3" role="status">
          <p className="text-sm">{t("timelineError")}</p>
          <Button variant="secondary" isLoading={loading} onClick={retry}>{t("timelineRetry")}</Button>
        </div>
      ) : (
        <Steps
            orientation="vertical"
            size="small"
            current={Math.max(0, timeline.events.length - 1)}
            className="mt-5 text-ui-fg-subtle"
            aria-label={t("timelineTitle")}
            styles={{ root: { fontFamily: "inherit", color: "inherit" }, itemContent: { color: "inherit", paddingBottom: 24 }, itemRail: { borderColor: "var(--order-progress-history)" } }}
            items={timeline.events.map((event, index) => {
              const isLatest = index === timeline.events.length - 1
              const state = !isLatest ? "history"
                : event.kind === "canceled" || event.kind === "fulfillment_canceled" ? "danger"
                : event.kind === "delivered" ? "success"
                : "current"
              const color = `var(--order-progress-${state})`
              return {
                key: event.id,
                status: state === "danger" ? "error" : state === "current" ? "process" : "finish",
                styles: {
                  icon: {
                    color: state === "current" ? "var(--order-progress-surface)" : color,
                    backgroundColor: state === "current" ? color : "var(--order-progress-surface)",
                    borderColor: color,
                  },
                },
                title: (
                  <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-ui-fg-base">
                    <span className="font-medium">{t(labels[event.kind])}</span>
                    {isLatest && <span className="text-xs text-ui-fg-subtle">{t("timelineLatest")}</span>}
                  </div>
                ),
                content: (
                  <div className="text-ui-fg-base" data-testid={`timeline-${event.kind}`} data-state={state}>
                    <time className="mt-1 block text-sm text-ui-fg-subtle" dateTime={event.occurred_at}>
                      {new Date(event.occurred_at).toLocaleString(locale, {
                        year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit",
                        hour12: false, timeZone: "Asia/Shanghai", timeZoneName: "short",
                      })}
                    </time>
                    {event.amount !== undefined && <p className="mt-1 text-sm">{convertToLocale({ amount: event.amount, currency_code: timeline.currency_code, locale })}</p>}
                  </div>
                ),
              }
            })}
          />
      )}
    </section>
  )
}
