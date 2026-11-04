import { withSEO } from "@lib/data/seo"
import { retrieveCart, retrieveCartVariants } from "@lib/data/cart"
import { retrieveCustomer } from "@lib/data/customer"
import CartTemplate from "@modules/cart/templates"
import { getCartCopy } from "@modules/cart/i18n"
import { Metadata } from "next"

type Props = { params: Promise<{ countryCode: string }> }
async function defaultPageMetadata({ params }: Props): Promise<Metadata> {
  return { title: getCartCopy((await params).countryCode).cart }
}

export default async function Cart({ params }: Props) {
  const [{ countryCode }, cart, customer] = await Promise.all([params, retrieveCart(), retrieveCustomer()])
  const variants = await retrieveCartVariants(cart)
  return <CartTemplate cart={cart} customer={customer} locale={countryCode} variants={variants} />
}

export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
