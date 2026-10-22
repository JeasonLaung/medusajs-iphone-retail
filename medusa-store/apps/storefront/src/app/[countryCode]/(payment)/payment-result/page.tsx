import { getCheckoutCopy } from "@modules/checkout/i18n"
import PaymentResult from "@modules/checkout/components/payment-result"
import type { Metadata } from "next"

export async function generateMetadata({ params }: { params: Promise<{ countryCode: string }> }): Promise<Metadata> {
  const { countryCode } = await params
  return { title: getCheckoutCopy(countryCode).paymentResultTitle, robots: { index: false, follow: false } }
}

export default async function PaymentResultPage({ params, searchParams }: {
  params: Promise<{ countryCode: string }>
  searchParams: Promise<{ cart_id?: string }>
}) {
  const [{ countryCode }, { cart_id }] = await Promise.all([params, searchParams])
  return <PaymentResult countryCode={countryCode} cartId={cart_id} />
}
