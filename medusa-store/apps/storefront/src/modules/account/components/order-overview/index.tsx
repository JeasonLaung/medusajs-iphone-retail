"use client"

import { LinkButton } from "@modules/account/components/ui"

import { Empty } from "antd"
import { useAccountCopy } from "@modules/account/use-account-copy"

import OrderCard from "../order-card"

import { HttpTypes } from "@medusajs/types"

const OrderOverview = ({ orders }: { orders: HttpTypes.StoreOrder[] }) => {
  const { t } = useAccountCopy()
  if (orders?.length) {
    return (
      <div className="flex flex-col gap-y-8 w-full">
        {orders.map((o) => (
          <div
            key={o.id}
            className="border-b border-gray-200 pb-6 last:pb-0 last:border-none"
          >
            <OrderCard order={o} />
          </div>
        ))}
      </div>
    )
  }

  return (
    <div
      className="w-full flex flex-col items-center gap-y-4"
      data-testid="no-orders-container"
    >
      <h2 className="text-large-semi">{t("noOrdersTitle")}</h2>
      <Empty description={t("noOrders")} />
      <div className="mt-4">
        <LinkButton href="/" data-testid="continue-shopping-button">
          {t("continueShopping")}
        </LinkButton>
      </div>
    </div>
  )
}

export default OrderOverview
