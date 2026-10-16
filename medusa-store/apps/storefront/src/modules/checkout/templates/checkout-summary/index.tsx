import Image from "next/image"
import { HttpTypes } from "@medusajs/types"
import { CheckoutStep } from "@lib/util/checkout-step"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import DiscountCode from "@modules/checkout/components/discount-code"
import { optionValueLabel } from "@modules/products/i18n"
import { getCheckoutCopy } from "../../i18n"
import styles from "../../styles.module.css"

export default function CheckoutSummary({ cart, step, locale }: { cart: HttpTypes.StoreCart; step: CheckoutStep; locale: string }) {
  const copy = getCheckoutCopy(locale)
  const money = (amount: number) => new Intl.NumberFormat(locale.endsWith("-zh") ? "zh-CN" : "en-IE", { style: "currency", currency: cart.currency_code }).format(amount)
  const shipping = cart.shipping_methods?.at(-1)
  const address = cart.shipping_address
  const totals = [[copy.subtotal, cart.item_subtotal ?? 0], [copy.shippingCost, shipping ? cart.shipping_subtotal ?? 0 : null], ...(cart.discount_total ? [[copy.discount, -cart.discount_total]] : []), [copy.tax, cart.tax_total ?? 0]] as [string, number | null][]
  return <aside className={styles.sidebar} aria-label={copy.summary}>
    <div className={styles.card}><h2 className={styles.secure} style={{ color: "inherit", marginBottom: 8, fontSize: 16 }}><svg width="18" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><rect x="5" y="10" width="14" height="11" rx="2" /><path d="M8 10V7a4 4 0 0 1 8 0v3" /></svg>{copy.secureCheckout}</h2><p className={styles.muted}>{copy.secureHelp}</p></div>
    <div className={styles.card}>
      <div className={styles.summaryTop}><strong>{copy.total} {money(cart.total ?? 0)}</strong><span className={styles.muted}>{copy.products}: {cart.items?.reduce((n, item) => n + item.quantity, 0)}</span></div>
      <details className={styles.orderItems} open={step !== "delivery"} key={step}><summary>{copy.summary}</summary>
        {cart.items?.map((item) => <div className={styles.lineItem} key={item.id}>
          <div className={styles.thumbnail}>{item.thumbnail && <Image src={item.thumbnail} alt={item.product_title ?? item.title} fill sizes="56px" />}</div>
          <div><strong>{item.product_title ?? item.title}</strong><p className={styles.muted}>{item.variant?.options?.length ? item.variant.options.map((option) => optionValueLabel(option.value, locale)).join(" · ") : item.variant_title}</p><p className={styles.muted}>{copy.quantity}: {item.quantity}</p></div>
          <span className={styles.linePrice}>{money(item.total ?? item.unit_price * item.quantity)}</span>
        </div>)}
        <div className={styles.totals}>{totals.map(([label, amount]) => <div key={label}><span>{label}</span><span>{amount === null ? copy.notSelected : money(amount)}</span></div>)}<div className={styles.total}><span>{copy.total}</span><strong data-testid="checkout-total">{money(cart.total ?? 0)}</strong></div></div>
      </details>
      <DiscountCode cart={cart} />
      {step !== "address" && address && <div className={styles.saved}>
        <div className={styles.savedTitle}><strong>{copy.contact}</strong><LocalizedClientLink href="/checkout?step=address">{copy.edit}</LocalizedClientLink></div>
        <p>{address.first_name} {address.last_name}</p><p>{cart.email}</p><p>{address.phone}</p><br />
        <p>{address.address_1}</p>{address.address_2 && <p>{address.address_2}</p>}<p>{address.postal_code} {address.city}</p><p>{cart.region?.countries?.find((country) => country.iso_2 === address.country_code)?.display_name ?? address.country_code?.toUpperCase()}</p>
      </div>}
      {(step === "payment" || step === "review") && shipping && <div className={styles.saved}><div className={styles.savedTitle}><strong>{copy.delivery}</strong><LocalizedClientLink href="/checkout?step=delivery">{copy.edit}</LocalizedClientLink></div><strong>{shipping.name}</strong><p>{money(shipping.amount ?? 0)}</p></div>}
    </div>
  </aside>
}
