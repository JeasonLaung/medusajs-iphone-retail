"use client"

import { setAddresses } from "@lib/data/cart"
import compareAddresses from "@lib/util/compare-addresses"
import { HttpTypes } from "@medusajs/types"
import { useParams } from "next/navigation"
import { useActionState, useState } from "react"
import BillingAddress from "../billing_address"
import ErrorMessage from "../error-message"
import ShippingAddress from "../shipping-address"
import { SubmitButton } from "../submit-button"
import { getCheckoutCopy } from "../../i18n"
import styles from "../../styles.module.css"

export default function Addresses({ cart, customer }: { cart: HttpTypes.StoreCart | null; customer: HttpTypes.StoreCustomer | null }) {
  const { countryCode } = useParams<{ countryCode: string }>()
  const copy = getCheckoutCopy(countryCode)
  const [sameAsBilling, setSameAsBilling] = useState(cart?.shipping_address && cart.billing_address ? compareAddresses(cart.shipping_address, cart.billing_address) : true)
  const [message, formAction] = useActionState(async (previous: string | null, data: FormData) => {
    if (String(data.get("email")).trim().toLowerCase() !== String(data.get("confirm_email")).trim().toLowerCase()) return copy.emailMismatch
    return setAddresses(previous, data)
  }, null)
  return <section aria-labelledby="contact-heading"><h2 id="contact-heading">{copy.contact}</h2><form action={formAction}>
    <ShippingAddress customer={customer} checked={sameAsBilling} onChange={() => setSameAsBilling(!sameAsBilling)} cart={cart} />
    {!sameAsBilling && <div className="mt-8"><h2>{copy.billing}</h2><BillingAddress cart={cart} /></div>}
    <SubmitButton className={styles.primary} data-testid="submit-address-button">{copy.continueDelivery}</SubmitButton>
    <ErrorMessage error={message} data-testid="address-error-message" />
  </form></section>
}
