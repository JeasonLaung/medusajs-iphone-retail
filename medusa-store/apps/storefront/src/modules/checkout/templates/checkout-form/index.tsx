import { listCartShippingMethods } from "@lib/data/fulfillment"
import { getCheckoutPaymentOptions } from "@lib/data/payment"
import { CheckoutStep } from "@lib/util/checkout-step"
import { HttpTypes } from "@medusajs/types"
import Addresses from "@modules/checkout/components/addresses"
import Payment from "@modules/checkout/components/payment"
import Review from "@modules/checkout/components/review"
import Shipping from "@modules/checkout/components/shipping"
import { getCheckoutCopy } from "../../i18n"
import styles from "../../styles.module.css"

export default async function CheckoutForm({ cart, customer, step, locale }: {
  cart: HttpTypes.StoreCart; customer: HttpTypes.StoreCustomer | null; step: CheckoutStep; locale: string
}) {
  if (step === "address") return <div className={styles.card}><Addresses cart={cart} customer={customer} /></div>
  if (step === "delivery") {
    const methods = await listCartShippingMethods(cart.id)
    return <div className={styles.card}>
      {!methods && <p role="alert" className={styles.error}>{getCheckoutCopy(locale).deliveryError}</p>}
      <Shipping cart={cart} availableShippingMethods={methods ?? []} />
    </div>
  }
  const methods = await getCheckoutPaymentOptions(cart)
  return <div className={`${styles.card} ${styles.payment}`}>
    <Payment cart={cart} availablePaymentMethods={methods.available} unavailablePaymentMethods={methods.unavailable} />
    {step === "review" && <Review cart={cart} />}
  </div>
}
