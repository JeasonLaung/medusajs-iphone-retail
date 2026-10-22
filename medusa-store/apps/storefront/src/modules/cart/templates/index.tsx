import { HttpTypes } from "@medusajs/types"
import CartLine from "../components/cart-line"
import CartInteractions from "../components/cart-interactions"
import Summary from "./summary"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { getCartCopy } from "../i18n"
import styles from "../styles.module.css"
import PageEntryScroll from "@modules/common/components/page-entry-scroll"

export default function CartTemplate({ cart, customer, locale, variants = {} }: {
  cart: HttpTypes.StoreCart | null
  customer: HttpTypes.StoreCustomer | null
  locale: string
  variants?: Record<string, HttpTypes.StoreProductVariant>
}) {
  const copy = getCartCopy(locale)
  const items = [...(cart?.items ?? [])].sort((a, b) => String(b.created_at ?? "").localeCompare(String(a.created_at ?? "")))
  const count = items.reduce((sum, item) => sum + item.quantity, 0)
  const benefits = [[copy.chosenTitle, copy.chosenText], [copy.totalsTitle, copy.totalsText], [copy.checkoutTitle, copy.checkoutText]]
  return <div className={styles.page}><div className={styles.container} data-testid="cart-container">
    <PageEntryScroll />
    {cart && items.length ? <CartInteractions><div className={styles.columns}>
      <section className={styles.card} aria-labelledby="cart-title">
        <div className={styles.cardHeading}><h1 id="cart-title">{copy.cart}</h1><span>{count} {copy.items}</span></div>
        {items.map((item) => <CartLine key={item.id} item={item} variant={variants[item.variant_id ?? ""]} currency={cart.currency_code} locale={locale} />)}
        <div className={styles.shopping}><LocalizedClientLink href="/store">{copy.continue}</LocalizedClientLink></div>
        {!customer && <p className={styles.signIn}>{copy.signInHelp}<LocalizedClientLink href="/account">{copy.signIn}</LocalizedClientLink></p>}
      </section>
      <Summary cart={cart} locale={locale} variants={variants} />
    </div></CartInteractions> : <section className={`${styles.card} ${styles.empty}`} data-testid="empty-cart-message">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true"><path d="M2 3h3l3 13h12l2-10H6" /><circle cx="9" cy="21" r="1" /><circle cx="19" cy="21" r="1" /></svg>
      <h1>{copy.empty}</h1><p className={styles.muted}>{copy.emptyHelp}</p><LocalizedClientLink href="/store" className={styles.primary}>{copy.continue}</LocalizedClientLink>
    </section>}
    <section className={`${styles.card} ${styles.benefits}`}>{benefits.map(([title, detail], index) => <div key={title}>
      <svg viewBox="0 0 32 32" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">{index === 0 ? <><rect x="9" y="2" width="14" height="28" rx="3" /><path d="M13 6h6M14 26h4" /></> : index === 1 ? <><rect x="6" y="3" width="20" height="26" rx="2" /><path d="M11 10h10M11 16h10M11 22h4" /></> : <><path d="M16 2 28 7v9c0 7-12 14-12 14S4 23 4 16V7Z" /><path d="m10 15 4 4 8-9" /></>}</svg>
      <h2>{title}</h2><p className={styles.muted}>{detail}</p>
    </div>)}</section>
  </div></div>
}
