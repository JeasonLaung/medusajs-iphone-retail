import { retrieveCart } from "@lib/data/cart"
import { retrieveCustomer } from "@lib/data/customer"
import { listProducts } from "@lib/data/products"
import CartTemplate from "@modules/cart/templates"
import { getCartCopy } from "@modules/cart/i18n"
import { HttpTypes } from "@medusajs/types"
import { Metadata } from "next"

type Props = { params: Promise<{ countryCode: string }> }
export async function generateMetadata({ params }: Props): Promise<Metadata> {
  return { title: getCartCopy((await params).countryCode).cart }
}

export default async function Cart({ params }: Props) {
  const [{ countryCode }, cart, customer] = await Promise.all([params, retrieveCart(), retrieveCustomer()])
  const ids = Array.from(new Set(cart?.items?.map((item) => item.product_id).filter((id): id is string => !!id) ?? []))
  const variants: Record<string, HttpTypes.StoreProductVariant> = {}
  if (ids.length && cart?.region_id) {
    const products = await listProducts({ regionId: cart.region_id, queryParams: { id: ids, limit: ids.length, fields: "id,*variants.options,+variants.metadata,+variants.inventory_quantity" } }).then(({ response }) => response.products).catch(() => [])
    products.forEach((product) => product.variants?.forEach((variant) => { variants[variant.id] = variant }))
  }
  return <CartTemplate cart={cart} customer={customer} locale={countryCode} variants={variants} />
}
