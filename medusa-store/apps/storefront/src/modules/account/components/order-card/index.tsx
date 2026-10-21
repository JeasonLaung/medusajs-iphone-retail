"use client"

import { useAccountCopy } from "@modules/account/use-account-copy"
import { getReviewCopy } from "@modules/products/components/product-reviews/i18n"

import { getOrderNumber } from "@lib/util/order-number"
import {
  getOrderStatusKey,
  getOrderStatusClasses,
  getOrderOriginalTotal,
} from "@lib/util/order-card-details"
import { Button } from "@modules/common/components/ui"
import { useMemo } from "react"

import Thumbnail from "@modules/products/components/thumbnail"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { convertToLocale } from "@lib/util/money"
import { HttpTypes } from "@medusajs/types"

type OrderCardProps = {
  order: HttpTypes.StoreOrder
}

const OrderCard = ({ order }: OrderCardProps) => {
  const { t, locale } = useAccountCopy()
  const statusKey = getOrderStatusKey(order)
  const originalTotal = getOrderOriginalTotal(order)
  const numberOfLines = useMemo(() => {
    return (
      order.items?.reduce((acc, item) => {
        return acc + item.quantity
      }, 0) ?? 0
    )
  }, [order])

  const numberOfProducts = useMemo(() => {
    return order.items?.length ?? 0
  }, [order])

  return (
    <div className="bg-white flex flex-col" data-testid="order-card">
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div className="min-w-0">
          <div className="mb-1 flex flex-wrap items-center gap-2">
            <span
              className={`inline-flex items-center rounded-md border px-2 py-0.5 text-small-regular ${getOrderStatusClasses(
                statusKey
              )}`}
              data-testid="order-status"
            >
              {t(statusKey)}
            </span>
            <span className="uppercase text-large-semi break-all">
              #
              <span data-testid="order-display-id">
                {getOrderNumber(order)}
              </span>
            </span>
          </div>
          <div className="flex flex-wrap items-center gap-y-1 divide-x divide-gray-200 text-small-regular text-ui-fg-base">
            <time
              className="pr-2"
              dateTime={new Date(order.created_at).toISOString()}
              data-testid="order-created-at"
            >
              {new Date(order.created_at).toLocaleString(locale, {
                year: "numeric",
                month: "2-digit",
                day: "2-digit",
                hour: "2-digit",
                minute: "2-digit",
                second: "2-digit",
                hour12: false,
                timeZone: "Asia/Shanghai",
                timeZoneName: "short",
              })}
            </time>
            <span className="pl-2">
              {t(numberOfLines === 1 ? "item" : "items", {
                count: numberOfLines,
              })}
            </span>
          </div>
        </div>
      </div>
      <div className="grid grid-cols-[repeat(auto-fill,112px)] small:grid-cols-[repeat(auto-fill,128px)] gap-3 my-3">
        {order.items?.slice(0, 3).map((i) => {
          return (
            <div
              key={i.id}
              className="min-w-0 flex flex-col gap-y-1.5"
              data-testid="order-item"
            >
              <Thumbnail
                thumbnail={i.thumbnail}
                images={[]}
                size="square"
                imageFit="contain"
              />
              <div className="flex items-start gap-1 text-small-regular text-ui-fg-base">
                <span
                  className="min-w-0 text-ui-fg-base font-semibold break-words"
                  data-testid="item-title"
                >
                  {i.title}
                </span>
                <span className="shrink-0">
                  x<span data-testid="item-quantity">{i.quantity}</span>
                </span>
              </div>
              <p
                className="text-small-regular text-ui-fg-base break-all"
                data-testid="item-sku"
              >
                {t("skuLabel")}{" "}
                {i.variant_sku || i.variant?.sku || t("notAvailable")}
              </p>
            </div>
          )
        })}
        {numberOfProducts > 4 && (
          <div className="w-full h-full flex flex-col items-center justify-center">
            <span className="text-small-regular text-ui-fg-base">
              + {numberOfLines - 4}
            </span>
            <span className="text-small-regular text-ui-fg-base">
              {t("more")}
            </span>
          </div>
        )}
      </div>
      <div className="flex flex-nowrap items-center justify-between gap-3 overflow-x-auto">
        <span className="flex shrink-0 items-center gap-x-2 whitespace-nowrap">
          <span
            className="text-xl font-semibold leading-7"
            data-testid="order-amount"
          >
            {convertToLocale({
              amount: order.total,
              currency_code: order.currency_code,
              locale,
            })}
          </span>
          {originalTotal !== null && (
            <del
              className="text-small-regular"
              data-testid="order-original-amount"
            >
              <span className="sr-only">{t("originalOrderTotal")} </span>
              {convertToLocale({
                amount: originalTotal,
                currency_code: order.currency_code,
                locale,
              })}
            </del>
          )}
        </span>
        <div className="ml-auto flex shrink-0 flex-nowrap items-center justify-end gap-3 whitespace-nowrap">
          {order.payment_status === "captured" &&
            order.status !== "canceled" && (
              <LocalizedClientLink
                href={`/account/orders/details/${order.id}#order-reviews`}
              >
                <Button variant="secondary">
                  {getReviewCopy(locale).reviewProducts}
                </Button>
              </LocalizedClientLink>
            )}
          <LocalizedClientLink href={`/account/orders/details/${order.id}`}>
            <Button data-testid="order-details-link" variant="secondary">
              {t("details")}
            </Button>
          </LocalizedClientLink>
        </div>
      </div>
    </div>
  )
}

export default OrderCard
