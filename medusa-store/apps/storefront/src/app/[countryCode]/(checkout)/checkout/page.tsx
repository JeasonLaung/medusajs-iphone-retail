import { retrieveCart } from "@lib/data/cart"
import { retrieveCustomer } from "@lib/data/customer"
import { checkoutStep } from "@lib/util/checkout-step"
import PaymentWrapper from "@modules/checkout/components/payment-wrapper"
import CheckoutProgress from "@modules/checkout/components/checkout-progress"
import CheckoutForm from "@modules/checkout/templates/checkout-form"
import CheckoutSummary from "@modules/checkout/templates/checkout-summary"
import { getCheckoutCopy } from "@modules/checkout/i18n"
import styles from "@modules/checkout/styles.module.css"
import { Metadata } from "next"
import { notFound, redirect } from "next/navigation"

type Props = { params: Promise<{ countryCode: string }>; searchParams: Promise<Record<string, string | string[] | undefined>> }
export async function generateMetadata({ params }: Props): Promise<Metadata> {
  return { title: getCheckoutCopy((await params).countryCode).checkout }
}

export default async function Checkout({ params, searchParams }: Props) {
  const [{ countryCode }, query, cart] = await Promise.all([params, searchParams, retrieveCart()])
  if (!cart) return notFound()
  if (!cart.items?.length) redirect(`/${countryCode}/cart`)
  const requested = typeof query.step === "string" ? query.step : undefined
  const step = checkoutStep(cart, requested)
  if (requested !== step) {
    const next = new URLSearchParams()
    Object.entries(query).forEach(([key, value]) => { if (typeof value === "string") next.set(key, value) })
    next.set("step", step)
    redirect(`/${countryCode}/checkout?${next}`)
  }
  // Customer addresses are only used by the address form.
  const customer = step === "address" ? await retrieveCustomer() : null
  return <div className={styles.page}>
    <CheckoutProgress step={step} />
    <div className={styles.columns}>
      <PaymentWrapper cart={cart}><CheckoutForm cart={cart} customer={customer} step={step} locale={countryCode} /></PaymentWrapper>
      <CheckoutSummary cart={cart} step={step} locale={countryCode} />
    </div>
  </div>
}
