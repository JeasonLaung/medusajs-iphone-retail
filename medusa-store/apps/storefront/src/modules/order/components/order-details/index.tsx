"use client"

import { Tag } from "antd"
import { useAccountCopy } from "@modules/account/use-account-copy"
import { AccountKey } from "@modules/account/i18n"

import { getOrderNumber } from "@lib/util/order-number"
import { HttpTypes } from "@medusajs/types"
import { Text } from "@modules/account/components/ui"

type OrderDetailsProps = {
  order: HttpTypes.StoreOrder
  showStatus?: boolean
}

const fulfillmentStatusKeys: Record<string, AccountKey> = {
  not_fulfilled: "fulfillmentNotFulfilled",
  partially_fulfilled: "fulfillmentPartiallyFulfilled",
  fulfilled: "fulfillmentFulfilled",
  partially_shipped: "fulfillmentPartiallyShipped",
  shipped: "fulfillmentShipped",
  partially_delivered: "fulfillmentPartiallyDelivered",
  delivered: "fulfillmentDelivered",
  canceled: "fulfillmentCanceled",
}

const paymentStatusKeys: Record<string, AccountKey> = {
  not_paid: "paymentNotPaid",
  awaiting: "paymentAwaiting",
  authorized: "paymentAuthorized",
  partially_authorized: "paymentPartiallyAuthorized",
  captured: "paymentCaptured",
  partially_captured: "paymentPartiallyCaptured",
  partially_refunded: "paymentPartiallyRefunded",
  refunded: "paymentRefunded",
  canceled: "paymentCanceled",
  requires_action: "paymentRequiresAction",
}

const OrderDetails = ({ order, showStatus }: OrderDetailsProps) => {
  const { t, locale } = useAccountCopy()
  const formatStatus = (str: string, keys: Record<string, AccountKey>) => {
    if (keys[str]) return t(keys[str])

    const formatted = str.split("_").join(" ")

    return formatted.slice(0, 1).toUpperCase() + formatted.slice(1)
  }

  return (
    <div>
      <Text>{t("confirmationEmail", { email: order.email || "" })}</Text>
      <Text className="mt-2">
        {t("orderDate")}{" "}
        <span data-testid="order-date">
          {new Date(order.created_at).toLocaleDateString(locale)}
        </span>
      </Text>
      <Text className="mt-2 text-ui-fg-interactive">
        {t("orderNumberLabel")}
        <span data-testid="order-id">{getOrderNumber(order)}</span>
      </Text>

      <div className="flex flex-wrap items-center text-compact-small gap-x-6 gap-y-3 mt-4">
        {showStatus && (
          <>
            <Text className="flex items-center gap-2">
              {t("orderStatus")}{" "}
              <Tag data-testid="order-status">
                {order.status === "canceled"
                  ? t("fulfillmentCanceled")
                  : formatStatus(
                      order.fulfillment_status,
                      fulfillmentStatusKeys
                    )}
              </Tag>
            </Text>
            <Text className="flex items-center gap-2">
              {t("paymentStatus")}{" "}
              <Tag data-testid="order-payment-status">
                {formatStatus(order.payment_status, paymentStatusKeys)}
              </Tag>
            </Text>
          </>
        )}
      </div>
    </div>
  )
}

export default OrderDetails
