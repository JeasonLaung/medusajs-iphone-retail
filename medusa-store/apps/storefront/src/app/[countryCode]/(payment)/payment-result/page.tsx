import { withSEO } from "@lib/data/seo"
import { getCheckoutCopy } from "@modules/checkout/i18n"
import PaymentResult from "@modules/checkout/components/payment-result"
import type { Metadata } from "next"

async function defaultPageMetadata({ params }: { params: Promise<{ countryCode: string }> }): Promise<Metadata> {
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

export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
