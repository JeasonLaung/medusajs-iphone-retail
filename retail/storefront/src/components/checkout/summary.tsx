
import type {CartView,CartContactDetail} from "../../lib/cart-client"
import type {Region} from "../../lib/regions"
import {formatCartMoney,isZeroAmount} from "../cart/types"
import type {CheckoutStep} from "./types"

import { optionValueLabel } from "../products/i18n"
import { getCheckoutCopy } from "./i18n"
import styles from "./styles.module.css"
import DiscountCode from './discount-code'
import type {PromotionState} from '../../lib/checkout-promotions'

export default function CheckoutSummary({ view,contacts,region,step,locale,onPromotionChange,onPromotionBlocked }: {view:CartView;contacts:CartContactDetail;region:Region;step:CheckoutStep;locale:string;onPromotionChange:(state:PromotionState)=>void;onPromotionBlocked:(blocked:boolean)=>void}) {
  const lines=new Map(view.amounts.lines.map(line=>[line.id,line]))
  const cart={item_subtotal:view.amounts.totals.item_subtotal,shipping_subtotal:view.amounts.totals.shipping_subtotal,discount_total:view.amounts.totals.discount_total,tax_total:view.amounts.totals.tax_total,total:view.amounts.totals.total,...contacts,currency_code:view.amounts.currency_code,shipping_methods:view.amounts.shipping_methods,region,items:view.items.map(item=>({...item,...lines.get(item.id)!,variant:{options:Object.values(item.option_values).map(value=>({value}))}}))}
  const copy = getCheckoutCopy(locale)
  const money = (amount: string) => formatCartMoney(amount,cart.currency_code,locale)
  const shipping = cart.shipping_methods?.at(-1)
  const address = cart.shipping_address
  const totals = [[copy.subtotal, cart.item_subtotal ?? "0"], [copy.shippingCost, shipping ? cart.shipping_subtotal ?? "0" : null], ...(!isZeroAmount(cart.discount_total) ? [[copy.discount, (cart.discount_total.startsWith("-")?cart.discount_total.slice(1):"-"+cart.discount_total)]] : []), [copy.tax, cart.tax_total ?? "0"]] as [string, string | null][]
  return <aside className={styles.sidebar} aria-label={copy.summary}>
    <div className={styles.card}><h2 className={styles.secure} style={{ color: "inherit", marginBottom: 8, fontSize: 16 }}><svg width="18" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><rect x="5" y="10" width="14" height="11" rx="2" /><path d="M8 10V7a4 4 0 0 1 8 0v3" /></svg>{copy.secureCheckout}</h2><p className={styles.muted}>{copy.secureHelp}</p></div>
    <div className={styles.card}>
      <div className={styles.summaryTop}><strong>{copy.total} {money(cart.total ?? "0")}</strong><span className={styles.muted}>{copy.products}: {cart.items?.reduce((n, item) => n + Number(item.quantity), 0)}</span></div>
      <details className={styles.orderItems} open={step !== "delivery"} key={step}><summary>{copy.summary}</summary>
        {cart.items?.map((item) => <div className={styles.lineItem} key={item.id}>
          <div className={styles.thumbnail}>{item.thumbnail && <img src={item.thumbnail} alt={item.product_title ?? item.title} style={{position:"absolute",height:"100%",width:"100%",inset:0}} />}</div>
          <div><strong>{item.product_title ?? item.title}</strong><p className={styles.muted}>{item.variant?.options?.length ? item.variant.options.map((option) => optionValueLabel(option.value, locale)).join(" · ") : item.variant_title}</p><p className={styles.muted}>{copy.quantity}: {item.quantity}</p></div>
          <span className={styles.linePrice}>{money(item.totals.total)}</span>
        </div>)}
        <div className={styles.totals}>{totals.map(([label, amount]) => <div key={label}><span>{label}</span><span>{amount === null ? copy.notSelected : money(amount)}</span></div>)}<div className={styles.total}><span>{copy.total}</span><strong data-testid="checkout-total">{money(cart.total ?? "0")}</strong></div></div>
      </details>
      <DiscountCode cartID={contacts.cart_id} version={contacts.version} segment={locale} onChange={onPromotionChange} onBlocked={onPromotionBlocked}/>
      {step !== "address" && address && <div className={styles.saved}>
        <div className={styles.savedTitle}><strong>{copy.contact}</strong><a href={`/${locale}/checkout?step=address`}>{copy.edit}</a></div>
        <p>{address.first_name} {address.last_name}</p><p>{cart.email}</p><p>{address.phone}</p><br />
        <p>{address.address_1}</p>{address.address_2 && <p>{address.address_2}</p>}<p>{address.postal_code} {address.city}</p><p>{cart.region?.countries?.find((country) => country.iso_2 === address.country_code)?.display_name ?? address.country_code?.toUpperCase()}</p>
      </div>}
      {(step === "payment" || step === "review") && shipping && <div className={styles.saved}><div className={styles.savedTitle}><strong>{copy.delivery}</strong><a href={`/${locale}/checkout?step=delivery`}>{copy.edit}</a></div><strong>{shipping.name}</strong><p>{money(shipping.amount ?? "0")}</p></div>}
    </div>
  </aside>
}
