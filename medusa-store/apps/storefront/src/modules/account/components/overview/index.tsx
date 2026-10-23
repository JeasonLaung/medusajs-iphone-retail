import { Container } from "@modules/account/components/ui"
import { Progress, Statistic, Empty } from "antd"
import { getLocale } from "@lib/data/locale-actions"
import { getAccountCopy } from "@modules/account/i18n"
import { getOrderNumber } from "@lib/util/order-number"

import ChevronDown from "@modules/common/icons/chevron-down"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { convertToLocale } from "@lib/util/money"
import { HttpTypes } from "@medusajs/types"

type OverviewProps = {
  customer: HttpTypes.StoreCustomer | null
  orders: HttpTypes.StoreOrder[] | null
}

const Overview = async ({ customer, orders }: OverviewProps) => {
  const { t, locale } = getAccountCopy(await getLocale())
  return (
    <div data-testid="overview-page-wrapper">
      <div className="hidden small:block">
        <div className="text-xl-semi flex flex-wrap gap-3 justify-between items-center mb-4">
          <span data-testid="welcome-message" data-value={customer?.first_name}>
            {t("hello", { name: customer?.first_name || "" })}
          </span>
          <span className="text-small-regular text-ui-fg-base">
            {t("signedIn")}{" "}
            <span
              className="font-semibold"
              data-testid="customer-email"
              data-value={customer?.email}
            >
              {customer?.email}
            </span>
          </span>
        </div>
        <div className="flex flex-col py-8 border-t border-gray-200">
          <div className="flex flex-col gap-y-4 h-full col-span-1 row-span-2 flex-1">
            <div className="flex items-start gap-x-16 mb-6">
              <div className="flex flex-col gap-y-4">
                <h3 className="text-large-semi">{t("profile")}</h3>
                <div className="flex items-end gap-x-2">
                  <div
                    data-testid="customer-profile-completion"
                    data-value={getProfileCompletion(customer)}
                  >
                    <Progress
                      type="circle"
                      size={72}
                      percent={getProfileCompletion(customer)}
                    />
                  </div>
                  <span className="uppercase text-base-regular text-ui-fg-subtle">
                    {t("completed")}
                  </span>
                </div>
              </div>

              <div className="flex flex-col gap-y-4">
                <h3 className="text-large-semi">{t("addresses")}</h3>
                <div className="flex items-end gap-x-2">
                  <div
                    data-testid="addresses-count"
                    data-value={customer?.addresses?.length || 0}
                  >
                    <Statistic value={customer?.addresses?.length || 0} />
                  </div>
                  <span className="uppercase text-base-regular text-ui-fg-subtle">
                    {t("saved")}
                  </span>
                </div>
              </div>
            </div>

            <div className="flex flex-col gap-y-4">
              <div className="flex items-center gap-x-2">
                <h3 className="text-large-semi">{t("recentOrders")}</h3>
              </div>
              <ul
                className="flex flex-col gap-y-4"
                data-testid="orders-wrapper"
              >
                {orders && orders.length > 0 ? (
                  orders.slice(0, 5).map((order) => {
                    return (
                      <li
                        key={order.id}
                        data-testid="order-wrapper"
                        data-value={order.id}
                      >
                        <LocalizedClientLink
                          href={`/account/orders/details/${order.id}`}
                        >
                          <Container className="bg-gray-50 flex justify-between items-center p-4">
                            <div className="grid grid-cols-3 grid-rows-2 text-small-regular gap-x-4 flex-1">
                              <span className="font-semibold">
                                {t("datePlaced")}
                              </span>
                              <span className="font-semibold">
                                {t("orderNumber")}
                              </span>
                              <span className="font-semibold">
                                {t("totalAmount")}
                              </span>
                              <span data-testid="order-created-date">
                                {new Date(order.created_at).toLocaleDateString(
                                  locale
                                )}
                              </span>
                              <span
                                data-testid="order-id"
                                data-value={getOrderNumber(order)}
                              >
                                #{getOrderNumber(order)}
                              </span>
                              <span data-testid="order-amount">
                                {convertToLocale({
                                  amount: order.total,
                                  currency_code: order.currency_code,
                                  locale,
                                })}
                              </span>
                            </div>
                            <span
                              className="flex items-center justify-between"
                              data-testid="open-order-button"
                            >
                              <span className="sr-only">
                                {t("goToOrder", {
                                  number: getOrderNumber(order),
                                })}
                              </span>
                              <ChevronDown className="-rotate-90" />
                            </span>
                          </Container>
                        </LocalizedClientLink>
                      </li>
                    )
                  })
                ) : (
                  <Empty
                    data-testid="no-orders-message"
                    description={t("noRecentOrders")}
                  />
                )}
              </ul>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

const getProfileCompletion = (customer: HttpTypes.StoreCustomer | null) => {
  let count = 0

  if (!customer) {
    return 0
  }

  if (customer.email) {
    count++
  }

  if (customer.first_name && customer.last_name) {
    count++
  }

  if (customer.phone) {
    count++
  }

  const billingAddress = customer.addresses?.find(
    (addr) => addr.is_default_billing
  )

  if (billingAddress) {
    count++
  }

  return (count / 4) * 100
}

export default Overview
