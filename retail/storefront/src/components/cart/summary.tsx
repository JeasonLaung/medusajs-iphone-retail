"use client"

import type {CartDisplay} from "./types"
import type {Variant} from "../../lib/products"
import {formatCartMoney,variantMaximum,isZeroAmount} from "./types"
import { variantInStock } from "../../lib/product-variants"
import LocalizedClientLink from "../products/localized-link"
import { getCartCopy } from "./i18n"
import styles from "./styles.module.css"
import { useCartInteractions } from "./interactions"

export default function Summary({ cart, locale, variants, checkoutHref=null }: { cart: CartDisplay; locale: string; variants: Record<string, Variant>; checkoutHref?:string|null }) {
  const copy = getCartCopy(locale)
  const { busy } = useCartInteractions()
  const money = (amount: string) => formatCartMoney(amount,cart.currency_code,locale)
  const unavailable = cart.items?.some((item) => {
    const variant = variants[item.variant_id ?? ""]
    return variant && (!variantInStock(variant) || (variantMaximum(variant) !== undefined && item.quantity > variantMaximum(variant)!))
  })
  return <aside className={`${styles.card} ${styles.summary}`} aria-label={copy.summary}>
    <div className={styles.summaryRows}>
      {cart.items?.map((item) => <div key={item.id}><span><strong className="font-bold">{item.product_title ?? item.title}</strong>{item.quantity > 1 ? ` × ${item.quantity}` : ""}</span><span>{money(item.subtotal)}</span></div>)}
      <div><span>{copy.shipping}</span><span data-testid="cart-shipping">{cart.shipping_methods?.length ? money(cart.shipping_subtotal) : copy.shippingPending}</span></div>
      {!isZeroAmount(cart.discount_total) && <div><span>{copy.discount}</span><span>− {money(cart.discount_total)}</span></div>}
      <div><span>{copy.taxes}</span><span>{money(cart.tax_total)}</span></div>
      <div className={styles.total}><span>{copy.total}</span><strong data-testid="cart-total" data-value={cart.total}>{money(cart.total)}</strong></div>
    </div>
    {unavailable || busy || !checkoutHref ? <><span className={styles.primary} aria-disabled="true" role="status">{busy ? copy.updating : copy.checkout}</span>{unavailable && <p role="alert" className={styles.error}>{copy.stockChanged}</p>}</> : <LocalizedClientLink segment={locale} href={checkoutHref!} className={styles.primary} data-testid="checkout-button">{copy.checkout}</LocalizedClientLink>}
    <p className={`${styles.muted} ${styles.checkoutHelp}`}>{copy.checkoutHelp}</p>
  </aside>
}
