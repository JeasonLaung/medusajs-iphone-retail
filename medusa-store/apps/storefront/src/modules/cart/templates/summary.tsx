"use client"

import { HttpTypes } from "@medusajs/types"
import { checkoutStep } from "@lib/util/checkout-step"
import { variantInStock } from "@lib/util/product-variants"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { getCartCopy } from "../i18n"
import styles from "../styles.module.css"
import { useCartInteractions } from "../components/cart-interactions"

export default function Summary({ cart, locale, variants }: { cart: HttpTypes.StoreCart; locale: string; variants: Record<string, HttpTypes.StoreProductVariant> }) {
  const copy = getCartCopy(locale)
  const { busy } = useCartInteractions()
  const money = (amount: number) => new Intl.NumberFormat(locale.endsWith("-zh") ? "zh-CN" : "en-IE", { style: "currency", currency: cart.currency_code }).format(amount)
  const unavailable = cart.items?.some((item) => {
    const variant = variants[item.variant_id ?? ""]
    return variant && (!variantInStock(variant) || (variant.manage_inventory && !variant.allow_backorder && typeof variant.inventory_quantity === "number" && item.quantity > variant.inventory_quantity))
  })
  return <aside className={`${styles.card} ${styles.summary}`} aria-label={copy.summary}>
    <div className={styles.summaryRows}>
      {cart.items?.map((item) => <div key={item.id}><span>{item.product_title ?? item.title}{item.quantity > 1 ? ` × ${item.quantity}` : ""}</span><span>{money(item.subtotal ?? item.unit_price * item.quantity)}</span></div>)}
      <div><span>{copy.shipping}</span><span data-testid="cart-shipping">{cart.shipping_methods?.length ? money(cart.shipping_subtotal ?? 0) : copy.shippingPending}</span></div>
      {!!cart.discount_total && <div><span>{copy.discount}</span><span>− {money(cart.discount_total)}</span></div>}
      <div><span>{copy.taxes}</span><span>{money(cart.tax_total ?? 0)}</span></div>
      <div className={styles.total}><span>{copy.total}</span><strong data-testid="cart-total" data-value={cart.total}>{money(cart.total ?? 0)}</strong></div>
    </div>
    {unavailable || busy ? <><span className={styles.primary} aria-disabled="true" role="status">{busy ? copy.updating : copy.checkout}</span>{unavailable && <p role="alert" className={styles.error}>{copy.stockChanged}</p>}</> : <LocalizedClientLink href={`/checkout?step=${checkoutStep(cart)}`} className={styles.primary} data-testid="checkout-button">{copy.checkout}</LocalizedClientLink>}
    <p className={`${styles.muted} ${styles.checkoutHelp}`}>{copy.checkoutHelp}</p>
  </aside>
}
