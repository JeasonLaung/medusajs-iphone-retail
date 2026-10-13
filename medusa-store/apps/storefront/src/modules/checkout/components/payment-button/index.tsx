"use client"

import { isManual, isMollie, isPaypal, isStripeLike } from "@lib/constants"
import { initiatePaymentSession, placeOrder } from "@lib/data/cart"
import {
  getActivePaymentSession,
  getMollieCheckoutUrl,
} from "@lib/util/payment-session"
import { MedusaNextPayPalAdapter } from "@easypayment/medusa-paypal-ui"
import { HttpTypes } from "@medusajs/types"
import { Button } from "@modules/common/components/ui"
import { useElements, useStripe } from "@stripe/react-stripe-js"
import { useParams, useSearchParams } from "next/navigation"
import React, { useState } from "react"
import ErrorMessage from "../error-message"

type PaymentButtonProps = {
  cart: HttpTypes.StoreCart
  "data-testid": string
}

const PaymentButton: React.FC<PaymentButtonProps> = ({
  cart,
  "data-testid": dataTestId,
}) => {
  const notReady =
    !cart ||
    !cart.shipping_address ||
    !cart.billing_address ||
    !cart.email ||
    (cart.shipping_methods?.length ?? 0) < 1

  const paymentSession = getActivePaymentSession(cart)

  switch (true) {
    case isPaypal(paymentSession?.provider_id):
      return <PayPalPaymentButton cart={cart} notReady={notReady} />
    case isMollie(paymentSession?.provider_id):
      return <MolliePaymentButton cart={cart} notReady={notReady} />
    case isStripeLike(paymentSession?.provider_id):
      return (
        <StripePaymentButton
          notReady={notReady}
          cart={cart}
          data-testid={dataTestId}
        />
      )
    case isManual(paymentSession?.provider_id):
      return (
        <ManualTestPaymentButton notReady={notReady} data-testid={dataTestId} />
      )
    default:
      return <Button disabled>Select a payment method</Button>
  }
}

const PayPalPaymentButton = ({
  cart,
  notReady,
}: {
  cart: HttpTypes.StoreCart
  notReady: boolean
}) => {
  const [error, setError] = useState<string | null>(null)
  if (notReady) return <Button disabled>Complete your delivery details</Button>
  return (
    <div className="max-w-[420px]">
      <MedusaNextPayPalAdapter
        cartId={cart.id}
        currencyCode={cart.currency_code}
        selectedProviderId={getActivePaymentSession(cart)?.provider_id}
        baseUrl={process.env.NEXT_PUBLIC_MEDUSA_BACKEND_URL!}
        publishableApiKey={process.env.NEXT_PUBLIC_MEDUSA_PUBLISHABLE_KEY}
        credentials="omit"
        onSuccess={async () => {
          await placeOrder(cart.id)
        }}
        onError={setError}
      />
      <ErrorMessage error={error} data-testid="paypal-payment-error-message" />
    </div>
  )
}

const MolliePaymentButton = ({
  cart,
  notReady,
}: {
  cart: HttpTypes.StoreCart
  notReady: boolean
}) => {
  const searchParams = useSearchParams()
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const session = getActivePaymentSession(cart)
  const pay = async () => {
    if (submitting || !session || !isMollie(session.provider_id)) return
    setSubmitting(true)
    setError(null)
    try {
      if (session.status === "authorized") {
        await placeOrder(cart.id)
        return
      }
      let url =
        Number(session.amount) === cart.total
          ? getMollieCheckoutUrl(session.data)
          : null
      if (!url) {
        const response = await initiatePaymentSession(cart, {
          provider_id: session.provider_id,
        })
        const nextSession = response.payment_collection.payment_sessions?.find(
          (entry) =>
            entry.provider_id === session.provider_id &&
            (entry.status === "pending" || entry.status === "requires_more")
        )
        url = getMollieCheckoutUrl(nextSession?.data)
      }
      if (!url)
        throw new Error("Could not open Mollie checkout. Please try again.")
      window.location.assign(url)
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Could not start your payment."
      )
      setSubmitting(false)
    }
  }
  return (
    <>
      {searchParams.get("payment_status") === "pending" && (
        <Button
          className="mb-3"
          disabled={notReady || submitting}
          size="large"
          onClick={() => {
            window.location.assign(
              `/api/mollie-return?cart_id=${encodeURIComponent(cart.id)}`
            )
          }}
        >
          Check payment status
        </Button>
      )}
      <Button
        disabled={notReady || submitting}
        isLoading={submitting}
        onClick={pay}
        size="large"
        data-testid="mollie-payment-button"
      >
        Continue to Mollie
      </Button>
      <ErrorMessage error={error} data-testid="mollie-payment-error-message" />
    </>
  )
}

const StripePaymentButton = ({
  cart,
  notReady,
  "data-testid": dataTestId,
}: {
  cart: HttpTypes.StoreCart
  notReady: boolean
  "data-testid"?: string
}) => {
  const [submitting, setSubmitting] = useState(false)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)

  const onPaymentCompleted = async () => {
    await placeOrder()
      .catch((err) => {
        setErrorMessage(err.message)
      })
      .finally(() => {
        setSubmitting(false)
      })
  }

  const stripe = useStripe()
  const elements = useElements()
  const { countryCode } = useParams()

  const disabled = !stripe || !elements ? true : false

  const handlePayment = async () => {
    if (!stripe || !elements || !cart) {
      return
    }

    setSubmitting(true)

    await stripe
      .confirmPayment({
        elements,
        confirmParams: {
          return_url: `${window.location.origin}/api/payment-return?cart_id=${cart.id}&country_code=${countryCode}`,
          payment_method_data: {
            billing_details: {
              name:
                cart.billing_address?.first_name +
                " " +
                cart.billing_address?.last_name,
              address: {
                city: cart.billing_address?.city ?? undefined,
                country: cart.billing_address?.country_code ?? undefined,
                line1: cart.billing_address?.address_1 ?? undefined,
                line2: cart.billing_address?.address_2 ?? undefined,
                postal_code: cart.billing_address?.postal_code ?? undefined,
                state: cart.billing_address?.province ?? undefined,
              },
              email: cart.email,
              phone: cart.billing_address?.phone ?? undefined,
            },
          },
        },
        // Only leave the site when the selected method actually requires it, so
        // card payments still complete inline.
        redirect: "if_required",
      })
      .then(({ error, paymentIntent }) => {
        if (error) {
          const pi = error.payment_intent

          if (
            (pi && pi.status === "requires_capture") ||
            (pi && pi.status === "succeeded")
          ) {
            onPaymentCompleted()
            return
          }

          setErrorMessage(error.message || null)
          setSubmitting(false)
          return
        }

        if (
          paymentIntent.status === "requires_capture" ||
          paymentIntent.status === "succeeded"
        ) {
          onPaymentCompleted()
          return
        }

        setSubmitting(false)
      })
  }

  return (
    <>
      <Button
        disabled={disabled || notReady}
        onClick={handlePayment}
        size="large"
        isLoading={submitting}
        data-testid={dataTestId}
      >
        Place order
      </Button>
      <ErrorMessage
        error={errorMessage}
        data-testid="stripe-payment-error-message"
      />
    </>
  )
}

const ManualTestPaymentButton = ({ notReady }: { notReady: boolean }) => {
  const [submitting, setSubmitting] = useState(false)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)

  const onPaymentCompleted = async () => {
    await placeOrder()
      .catch((err) => {
        setErrorMessage(err.message)
      })
      .finally(() => {
        setSubmitting(false)
      })
  }

  const handlePayment = () => {
    setSubmitting(true)

    onPaymentCompleted()
  }

  return (
    <>
      <Button
        disabled={notReady}
        isLoading={submitting}
        onClick={handlePayment}
        size="large"
        data-testid="submit-order-button"
      >
        Place order
      </Button>
      <ErrorMessage
        error={errorMessage}
        data-testid="manual-payment-error-message"
      />
    </>
  )
}

export default PaymentButton
