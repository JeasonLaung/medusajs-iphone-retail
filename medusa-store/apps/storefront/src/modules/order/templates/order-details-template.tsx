"use client"

import { useAccountCopy } from "@modules/account/use-account-copy"

import { XMark } from "@medusajs/icons"
import { HttpTypes } from "@medusajs/types"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import AfterSales from "@modules/order/components/after-sales"
import OrderReviews from "@modules/order/components/order-reviews"
import Items from "@modules/order/components/items"
import OrderDetails from "@modules/order/components/order-details"
import OrderTimelineSteps from "@modules/order/components/order-timeline"
import type { OrderTimeline } from "types/order-timeline"
import OrderSummary from "@modules/order/components/order-summary"
import ShippingDetails from "@modules/order/components/shipping-details"
import React from "react"

type OrderDetailsTemplateProps = {
  order: HttpTypes.StoreOrder
  timeline?: OrderTimeline | null
}

const OrderDetailsTemplate: React.FC<OrderDetailsTemplateProps> = ({
  order,
  timeline = null,
}) => {
  const { t } = useAccountCopy()
  return (
    <div className="flex flex-col justify-center gap-y-4">
      <div className="flex gap-2 justify-between items-center">
        <h1 className="text-2xl-semi">{t("orderDetails")}</h1>
        <LocalizedClientLink
          href="/account/orders"
          className="flex gap-2 items-center text-ui-fg-subtle hover:text-ui-fg-base"
          data-testid="back-to-overview-button"
        >
          <XMark />
          {t("backOverview")}
        </LocalizedClientLink>
      </div>
      <div
        className="flex flex-col gap-4 h-full bg-white w-full"
        data-testid="order-details-container"
      >
        <OrderDetails order={order} showStatus />
        <div className="grid grid-cols-1 gap-8 small:grid-cols-[minmax(0,1fr)_260px]">
          <div className="min-w-0 flex flex-col gap-4">
            <Items order={order} />
            <ShippingDetails order={order} />
            <OrderSummary order={order} />
            {order.status !== "canceled" && (
              <>
                <OrderReviews key={order.id} order={order} />
                <AfterSales order={order} />
              </>
            )}
          </div>
          <aside className="order-first min-w-0 small:order-last">
            <OrderTimelineSteps key={order.id} orderId={order.id} initialTimeline={timeline} />
          </aside>
        </div>
      </div>
    </div>
  )
}

export default OrderDetailsTemplate
