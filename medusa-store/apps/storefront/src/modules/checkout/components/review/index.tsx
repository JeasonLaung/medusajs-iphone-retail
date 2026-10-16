"use client"

import PaymentButton from "../payment-button"
import { useParams } from "next/navigation"
import { HttpTypes } from "@medusajs/types"
import { getActivePaymentSession } from "@lib/util/payment-session"
import { getCheckoutCopy } from "../../i18n"
import styles from "../../styles.module.css"

export default function Review({ cart }: { cart: HttpTypes.StoreCart }) {
  const { countryCode } = useParams<{ countryCode: string }>()
  const copy = getCheckoutCopy(countryCode)
  const giftCards = (cart as unknown as { gift_cards?: unknown[] }).gift_cards
  if (!cart.shipping_address || !cart.shipping_methods?.length || (!getActivePaymentSession(cart) && !(giftCards?.length && cart.total === 0))) return null
  return <section aria-label={copy.review}><p className={styles.terms}>{copy.terms}</p><PaymentButton cart={cart} data-testid="submit-order-button" /></section>
}
