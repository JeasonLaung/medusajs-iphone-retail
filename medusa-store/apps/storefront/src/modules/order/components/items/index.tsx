"use client"

import { Table } from "antd"
import type { HttpTypes } from "@medusajs/types"
import { useAccountCopy } from "@modules/account/use-account-copy"
import Thumbnail from "@modules/products/components/thumbnail"
import { getOrderItemThumbnail } from "@lib/util/order-item-thumbnail"
import LineItemOptions from "@modules/common/components/line-item-options"
import LineItemPrice from "@modules/common/components/line-item-price"
import LineItemUnitPrice from "@modules/common/components/line-item-unit-price"

export default function Items({ order }: { order: HttpTypes.StoreOrder }) {
  const { t } = useAccountCopy()
  return (
    <div data-testid="products-table">
      <Table<HttpTypes.StoreOrderLineItem>
        rowKey="id"
        pagination={false}
        showHeader={false}
        size="small"
        dataSource={[...(order.items ?? [])].sort((a, b) =>
          (a.created_at ?? "") > (b.created_at ?? "") ? -1 : 1
        )}
        onRow={() =>
          ({
            "data-testid": "product-row",
          } as React.HTMLAttributes<HTMLTableRowElement>)
        }
        columns={[
          {
            key: "image",
            width: 80,
            render: (_, item) => (
              <div className="w-16">
                <Thumbnail thumbnail={getOrderItemThumbnail(item)} size="square" />
              </div>
            ),
          },
          {
            key: "product",
            title: t("items"),
            render: (_, item) => (
              <div className="min-w-0 break-words">
                <span className="font-semibold" data-testid="product-name">
                  {item.product_title}
                </span>
                <LineItemOptions
                  variant={item.variant}
                  data-testid="product-variant"
                />
              </div>
            ),
          },
          {
            key: "price",
            align: "right",
            title: t("total"),
            render: (_, item) => (
              <div className="flex flex-col items-end">
                <span className="flex gap-1">
                  <span data-testid="product-quantity">{item.quantity}</span>x{" "}
                  <LineItemUnitPrice
                    item={item}
                    style="tight"
                    currencyCode={order.currency_code}
                  />
                </span>
                <LineItemPrice
                  item={item}
                  style="tight"
                  currencyCode={order.currency_code}
                />
              </div>
            ),
          },
        ]}
      />
    </div>
  )
}
