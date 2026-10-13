"use client"
import { RadioGroup } from "@headlessui/react"
import {
  isMollie,
  isPaypal,
  isStripeLike,
  paymentInfoMap,
} from "@lib/constants"
import { getActivePaymentSession } from "@lib/util/payment-session"
import { initiatePaymentSession } from "@lib/data/cart"
import { CheckCircleSolid, CreditCard } from "@medusajs/icons"
import ErrorMessage from "@modules/checkout/components/error-message"
import PaymentContainer, {
  StripePaymentContainer,
} from "@modules/checkout/components/payment-container"
import Divider from "@modules/common/components/divider"
import {
  Button,
  Container,
  Heading,
  Text,
  clx,
} from "@modules/common/components/ui"
import { HttpTypes } from "@medusajs/types"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { useCallback, useEffect, useState } from "react"

const Payment = ({
  cart,
  availablePaymentMethods,
  unavailablePaymentMethods = [],
}: {
  cart: HttpTypes.StoreCart
  availablePaymentMethods: { id: string }[]
  unavailablePaymentMethods?: { id: string; title: string }[]
}) => {
  const activeSession = getActivePaymentSession(cart)

  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [paymentComplete, setPaymentComplete] = useState(false)
  const [selectedPaymentMethod, setSelectedPaymentMethod] = useState(
    activeSession?.provider_id ?? ""
  )

  const searchParams = useSearchParams()
  const router = useRouter()
  const pathname = usePathname()

  const isOpen = searchParams.get("step") === "payment"

  const setPaymentMethod = async (method: string) => {
    if (isLoading) return
    setError(null)
    setSelectedPaymentMethod(method)
    if (isStripeLike(method)) {
      setIsLoading(true)
      try {
        await initiatePaymentSession(cart, { provider_id: method })
      } catch (err) {
        setError(
          err instanceof Error ? err.message : "Could not set up payment."
        )
      } finally {
        setIsLoading(false)
      }
    }
  }

  const paidByGiftcard = !!(
    (cart as unknown as Record<string, unknown>)?.gift_cards &&
    ((cart as unknown as Record<string, unknown>)?.gift_cards as unknown[])
      ?.length > 0 &&
    cart?.total === 0
  )

  const paymentReady =
    (activeSession && (cart?.shipping_methods?.length ?? 0) !== 0) ||
    paidByGiftcard

  const createQueryString = useCallback(
    (name: string, value: string) => {
      const params = new URLSearchParams(searchParams)
      params.set(name, value)

      return params.toString()
    },
    [searchParams]
  )

  const handleEdit = () => {
    router.push(pathname + "?" + createQueryString("step", "payment"), {
      scroll: false,
    })
  }

  const handleSubmit = async () => {
    if (
      isLoading ||
      (!paidByGiftcard &&
        !availablePaymentMethods.some(
          (method) => method.id === selectedPaymentMethod
        ))
    )
      return
    setIsLoading(true)
    try {
      const shouldInputPaymentDetails =
        isStripeLike(selectedPaymentMethod) && !activeSession

      const checkActiveSession =
        activeSession?.provider_id === selectedPaymentMethod &&
        Number(activeSession.amount) === cart.total

      if (!paidByGiftcard && !checkActiveSession) {
        await initiatePaymentSession(cart, {
          provider_id: selectedPaymentMethod,
        })
      }

      if (!shouldInputPaymentDetails) {
        return router.push(
          pathname + "?" + createQueryString("step", "review"),
          {
            scroll: false,
          }
        )
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => {
    setError(null)
  }, [isOpen])

  useEffect(() => {
    if (
      activeSession?.provider_id &&
      availablePaymentMethods.some(
        (method) => method.id === activeSession.provider_id
      )
    ) {
      setSelectedPaymentMethod(activeSession.provider_id)
    }
  }, [activeSession?.id, activeSession?.provider_id, availablePaymentMethods])

  const returnedStatus = searchParams.get("payment_status")
  const returnMessage =
    returnedStatus === "failed"
      ? "Payment was not completed. Please try again or choose another payment method."
      : returnedStatus === "pending"
      ? "Your payment is still being confirmed. You can check it again below."
      : returnedStatus === "invalid"
      ? "We could not match this payment to your current cart. Please reopen checkout."
      : null

  return (
    <div className="bg-white">
      <div className="flex flex-row items-center justify-between mb-6">
        <Heading
          level="h2"
          className={clx(
            "flex flex-row text-3xl-regular gap-x-2 items-baseline",
            {
              "opacity-50 pointer-events-none select-none":
                !isOpen && !paymentReady,
            }
          )}
        >
          Payment
          {!isOpen && paymentReady && <CheckCircleSolid />}
        </Heading>
        {!isOpen && paymentReady && (
          <Text>
            <button
              onClick={handleEdit}
              className="text-ui-fg-interactive hover:text-ui-fg-interactive-hover"
              data-testid="edit-payment-button"
            >
              Edit
            </button>
          </Text>
        )}
      </div>
      <div>
        <div className={isOpen ? "block" : "hidden"}>
          {!paidByGiftcard && (
            <>
              <RadioGroup
                value={selectedPaymentMethod}
                onChange={(value: string) => setPaymentMethod(value)}
              >
                {availablePaymentMethods.map((paymentMethod) => (
                  <div key={paymentMethod.id}>
                    {isStripeLike(paymentMethod.id) ? (
                      <StripePaymentContainer
                        paymentProviderId={paymentMethod.id}
                        selectedPaymentOptionId={selectedPaymentMethod}
                        paymentInfoMap={paymentInfoMap}
                        setError={setError}
                        setPaymentComplete={setPaymentComplete}
                        disabled={isLoading}
                      />
                    ) : (
                      <PaymentContainer
                        paymentInfoMap={paymentInfoMap}
                        paymentProviderId={paymentMethod.id}
                        selectedPaymentOptionId={selectedPaymentMethod}
                        disabled={isLoading}
                      />
                    )}
                  </div>
                ))}
                {unavailablePaymentMethods.map((method) => (
                  <PaymentContainer
                    key={method.id}
                    paymentInfoMap={paymentInfoMap}
                    paymentProviderId={method.id}
                    selectedPaymentOptionId={null}
                    disabled
                  >
                    <Text className="text-ui-fg-subtle">
                      Currently unavailable. Please choose another payment
                      method.
                    </Text>
                  </PaymentContainer>
                ))}
              </RadioGroup>
            </>
          )}

          {paidByGiftcard && (
            <div className="flex flex-col w-1/3">
              <Text className="txt-medium-plus text-ui-fg-base mb-1">
                Payment method
              </Text>
              <Text
                className="txt-medium text-ui-fg-subtle"
                data-testid="payment-method-summary"
              >
                Gift card
              </Text>
            </div>
          )}

          <ErrorMessage
            error={error || returnMessage}
            data-testid="payment-method-error-message"
          />

          {returnedStatus === "pending" &&
            isMollie(activeSession?.provider_id) && (
              <Button
                size="large"
                className="mt-4"
                onClick={() =>
                  window.location.assign(
                    `/api/mollie-return?cart_id=${encodeURIComponent(cart.id)}`
                  )
                }
              >
                Check payment status
              </Button>
            )}

          <Button
            size="large"
            className="mt-6"
            onClick={handleSubmit}
            isLoading={isLoading}
            disabled={
              (isStripeLike(selectedPaymentMethod) && !paymentComplete) ||
              (!selectedPaymentMethod && !paidByGiftcard) ||
              isLoading ||
              (!paidByGiftcard &&
                !availablePaymentMethods.some(
                  (method) => method.id === selectedPaymentMethod
                ))
            }
            data-testid="submit-payment-button"
          >
            {!activeSession && isStripeLike(selectedPaymentMethod)
              ? "Enter payment details"
              : "Continue to review"}
          </Button>
        </div>

        <div className={isOpen ? "hidden" : "block"}>
          {cart && paymentReady && activeSession ? (
            <div className="flex items-start gap-x-1 w-full">
              <div className="flex flex-col w-1/3">
                <Text className="txt-medium-plus text-ui-fg-base mb-1">
                  Payment method
                </Text>
                <Text
                  className="txt-medium text-ui-fg-subtle"
                  data-testid="payment-method-summary"
                >
                  {paymentInfoMap[activeSession?.provider_id]?.title ||
                    activeSession?.provider_id}
                </Text>
              </div>
              <div className="flex flex-col w-1/3">
                <Text className="txt-medium-plus text-ui-fg-base mb-1">
                  Payment details
                </Text>
                <div
                  className="flex gap-2 txt-medium text-ui-fg-subtle items-center"
                  data-testid="payment-details-summary"
                >
                  <Container className="flex items-center h-7 w-fit p-2 bg-ui-button-neutral-hover">
                    {paymentInfoMap[selectedPaymentMethod]?.icon || (
                      <CreditCard />
                    )}
                  </Container>
                  <Text>
                    {isMollie(activeSession.provider_id)
                      ? "You will continue to Mollie to pay securely."
                      : isPaypal(activeSession.provider_id)
                      ? "Pay securely with PayPal below."
                      : "Payment details confirmed"}
                  </Text>
                </div>
              </div>
            </div>
          ) : paidByGiftcard ? (
            <div className="flex flex-col w-1/3">
              <Text className="txt-medium-plus text-ui-fg-base mb-1">
                Payment method
              </Text>
              <Text
                className="txt-medium text-ui-fg-subtle"
                data-testid="payment-method-summary"
              >
                Gift card
              </Text>
            </div>
          ) : null}
        </div>
      </div>
      <Divider className="mt-8" />
    </div>
  )
}

export default Payment
