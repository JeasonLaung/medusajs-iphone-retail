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
import ErrorMessage from "@modules/checkout/components/error-message"
import PaymentContainer, {
  StripePaymentContainer,
} from "@modules/checkout/components/payment-container"
import { Button } from "@modules/common/components/ui"
import { HttpTypes } from "@medusajs/types"
import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation"
import { useCallback, useEffect, useState } from "react"
import { getCheckoutCopy, paymentTitle } from "../../i18n"
import styles from "../../styles.module.css"

const Payment = ({
  cart,
  availablePaymentMethods,
  unavailablePaymentMethods = [],
}: {
  cart: HttpTypes.StoreCart
  availablePaymentMethods: { id: string }[]
  unavailablePaymentMethods?: { id: string; title: string }[]
}) => {
  const { countryCode } = useParams<{ countryCode: string }>()
  const copy = getCheckoutCopy(countryCode)
  const localizedPaymentInfo = Object.fromEntries(Object.entries(paymentInfoMap).map(([id, value]) => [id, { ...value, title: paymentTitle(id, countryCode, value.title) }]))
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
          err instanceof Error ? err.message : copy.paymentSetupError
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
      ? copy.paymentFailed
      : returnedStatus === "pending"
      ? copy.paymentPending
      : returnedStatus === "invalid"
      ? copy.paymentInvalid
      : null

  return <section aria-labelledby="payment-heading">
    <div className="flex items-center justify-between gap-4 mb-6"><h2 id="payment-heading" style={{ marginBottom: 0 }}>{copy.paymentMethods}</h2>
      {!isOpen && paymentReady && <button type="button" onClick={handleEdit} className="text-sm text-blue-600" data-testid="edit-payment-button">{copy.edit}</button>}
    </div>
    {isOpen ? <>
      <h3 className={styles.sectionLabel}>{copy.payNow}</h3>
      {!paidByGiftcard && <RadioGroup value={selectedPaymentMethod} onChange={(value: string) => setPaymentMethod(value)} aria-label={copy.paymentMethods}>
        {availablePaymentMethods.map((method) => isStripeLike(method.id) ? <StripePaymentContainer key={method.id} paymentProviderId={method.id} selectedPaymentOptionId={selectedPaymentMethod} paymentInfoMap={localizedPaymentInfo} setError={setError} setPaymentComplete={setPaymentComplete} disabled={isLoading} /> : <PaymentContainer key={method.id} paymentProviderId={method.id} selectedPaymentOptionId={selectedPaymentMethod} paymentInfoMap={localizedPaymentInfo} disabled={isLoading} />)}
        {unavailablePaymentMethods.map((method) => <PaymentContainer key={method.id} paymentProviderId={method.id} selectedPaymentOptionId={null} paymentInfoMap={localizedPaymentInfo} disabled><p className={styles.muted}>{copy.paymentUnavailable}</p></PaymentContainer>)}
      </RadioGroup>}
      {paidByGiftcard && <p>{copy.giftCard}</p>}
      {!availablePaymentMethods.length && !paidByGiftcard && <p role="alert" className={styles.error}>{copy.noPayment}</p>}
      <ErrorMessage error={error || returnMessage} data-testid="payment-method-error-message" />
      {returnedStatus === "pending" && isMollie(activeSession?.provider_id) && <Button className="my-4" onClick={() => window.location.assign(`/api/mollie-return?cart_id=${encodeURIComponent(cart.id)}`)}>{copy.checkPayment}</Button>}
      <Button className="mt-6" size="large" onClick={handleSubmit} isLoading={isLoading} disabled={(isStripeLike(selectedPaymentMethod) && !paymentComplete) || (!selectedPaymentMethod && !paidByGiftcard) || isLoading || (!paidByGiftcard && !availablePaymentMethods.some((method) => method.id === selectedPaymentMethod))} data-testid="submit-payment-button">{!activeSession && isStripeLike(selectedPaymentMethod) ? copy.enterPayment : copy.reviewPayment}</Button>
    </> : <div>
      <p className="font-semibold mb-2" data-testid="payment-method-summary">{activeSession ? localizedPaymentInfo[activeSession.provider_id]?.title ?? activeSession.provider_id : paidByGiftcard ? copy.giftCard : copy.selectPayment}</p>
      <p className={styles.muted}>{activeSession && isMollie(activeSession.provider_id) ? copy.mollieHelp : activeSession && isPaypal(activeSession.provider_id) ? copy.paypalHelp : copy.paymentConfirmed}</p>
      {returnMessage && <p role="alert" className={styles.error}>{returnMessage}</p>}
    </div>}
  </section>
}

export default Payment
