"use client"

import { useAccountCopy } from "@modules/account/use-account-copy"
import { getCheckoutCopy, paymentTitle } from "@modules/checkout/i18n"

import { Container, Heading, Text } from "@modules/common/components/ui"

import { isStripeLike, paymentInfoMap } from "@lib/constants"
import Divider from "@modules/common/components/divider"
import { convertToLocale } from "@lib/util/money"
import { HttpTypes } from "@medusajs/types"

type PaymentDetailsProps = {
  order: HttpTypes.StoreOrder
}

const PaymentDetails = ({ order }: PaymentDetailsProps) => {
  const { t, locale } = useAccountCopy()
  const copy = getCheckoutCopy(locale)
  const payment = order.payment_collections?.[0]?.payments?.[0]
  const paymentInfo = payment ? paymentInfoMap[payment.provider_id] : undefined

  return (
    <div>
      <Heading level="h2" className="flex flex-row text-3xl-regular my-6">
        {t("paymentHeading")}
      </Heading>
      <div>
        {payment && (
          <div className="flex items-start gap-x-1 w-full">
            <div className="flex flex-col w-1/3">
              <Text className="txt-medium-plus text-ui-fg-base mb-1">
                {copy.paymentMethod}
              </Text>
              <Text
                className="txt-medium text-ui-fg-subtle"
                data-testid="payment-method"
              >
                {paymentTitle(payment.provider_id, locale, paymentInfo?.title)}
              </Text>
            </div>
            <div className="flex flex-col w-2/3">
              <Text className="txt-medium-plus text-ui-fg-base mb-1">
                {copy.paymentDetails}
              </Text>
              <div className="flex gap-2 txt-medium text-ui-fg-subtle items-center">
                <Container aria-hidden="true" className="flex items-center h-7 w-fit p-2 bg-ui-button-neutral-hover">
                  {paymentInfo?.icon}
                </Container>
                <Text data-testid="payment-amount">
                  {isStripeLike(payment.provider_id) && payment.data?.card_last4
                    ? `**** **** **** ${payment.data.card_last4}`
                    : t("paymentAmountAt", {
                        amount: convertToLocale({
                          amount: payment.amount,
                          currency_code: order.currency_code,
                          locale,
                        }),
                        date: new Date(
                        payment.created_at ?? ""
                        ).toLocaleString(locale),
                      })}
                </Text>
              </div>
            </div>
          </div>
        )}
      </div>

      <Divider className="mt-8" />
    </div>
  )
}

export default PaymentDetails
