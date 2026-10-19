"use client"

import { Minus, Plus } from "@medusajs/icons"
import Image from "next/image"
import { useEffect, useId, useState } from "react"
import { HttpTypes } from "@medusajs/types"
import { deleteLineItem, updateLineItem } from "@lib/data/cart"
import { variantOptions, variantSlug, variantInStock } from "@lib/util/product-variants"
import { optionValueLabel } from "@modules/products/i18n"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { getCartCopy } from "../../i18n"
import styles from "../../styles.module.css"
import { useCartInteractions } from "../cart-interactions"

export default function CartLine({ item, variant, currency, locale }: { item: HttpTypes.StoreCartLineItem; variant?: HttpTypes.StoreProductVariant; currency: string; locale: string }) {
  const copy = getCartCopy(locale)
  const { busy, setBusy } = useCartInteractions()
  const [pending, setPending] = useState<"quantity" | "remove" | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [quantity, setQuantity] = useState(String(item.quantity))
  const quantityId = useId()
  const enteredQuantity = Number(quantity)
  const stepQuantity = Number.isSafeInteger(enteredQuantity) && enteredQuantity >= 1 ? enteredQuantity : item.quantity
  useEffect(() => setQuantity(String(item.quantity)), [item.quantity])
  const maximum = variant?.manage_inventory && !variant.allow_backorder && typeof variant.inventory_quantity === "number" ? Math.max(0, variant.inventory_quantity) : undefined
  const unavailable = !!variant && (!variantInStock(variant) || (maximum !== undefined && item.quantity > maximum))
  const url = `/products/${variantSlug(variant) ?? item.product_handle}`
  const values = variant ? Object.values(variantOptions(variant)) : (item.variant_title ?? "").split(" / ").filter(Boolean)
  const grade = values.find((value) => value === "S" || value === "A")
  const battery = values.find((value) => /^\d+%$/.test(value))
  const description = values.filter((value) => value !== grade && value !== battery).map((value) => optionValueLabel(value, locale)).join(" · ")
  const money = (amount: number) => new Intl.NumberFormat(locale.endsWith("-zh") ? "zh-CN" : "en-IE", { style: "currency", currency }).format(amount)

  const changeQuantity = async (next: number) => {
    if (busy || pending) return
    if (!Number.isSafeInteger(next) || next < 1 || (maximum !== undefined && next > maximum)) { setQuantity(String(item.quantity)); setError(copy.stockChanged); return }
    if (next === item.quantity) { setQuantity(String(item.quantity)); return }
    setPending("quantity")
    setBusy(true)
    setError(null)
    setQuantity(String(next))
    try { await updateLineItem({ lineId: item.id, quantity: next }) }
    catch { setQuantity(String(item.quantity)); setError(copy.updateError) }
    finally { setPending(null); setBusy(false) }
  }
  const remove = async () => {
    if (busy || pending) return
    setPending("remove")
    setBusy(true)
    setError(null)
    try { await deleteLineItem(item.id) }
    catch { setError(copy.removeError); setPending(null) }
    finally { setBusy(false) }
  }

  return <article className={styles.line} data-testid="product-row" aria-busy={!!pending}>
    <LocalizedClientLink href={url} className={styles.thumbnail} aria-label={item.product_title ?? item.title}>{item.thumbnail && <Image src={item.thumbnail} alt={item.product_title ?? item.title} fill sizes="64px" />}</LocalizedClientLink>
    <div>
      <LocalizedClientLink href={url} className={styles.title} data-testid="product-title">{item.product_title ?? item.title}</LocalizedClientLink>
      <div className={styles.details}>{grade && <span className={styles.grade}>{optionValueLabel(grade, locale)}</span>}<span data-testid="product-variant">{description}</span>{battery && <span className={styles.battery}>· {copy.battery}: {battery}</span>}</div>
      <div className={styles.lineControls}>
      <div className={styles.quantity}>
        <label htmlFor={quantityId}>{copy.quantity}:</label>
        <div className={styles.quantityStepper} role="group" aria-label={copy.quantity} onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) changeQuantity(Number(quantity)) }}>
          <button type="button" aria-label={copy.decreaseQuantity} disabled={busy || !!pending || stepQuantity <= 1} onClick={() => changeQuantity(stepQuantity - 1)}><Minus aria-hidden="true" width={20} height={20} className="inline-block align-middle shrink-0" /></button>
          <input id={quantityId} type="number" aria-label={copy.quantity} min={1} max={maximum} step={1} value={quantity} disabled={busy || !!pending} onChange={(event) => setQuantity(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") event.currentTarget.blur() }} data-testid="product-select-button" />
          <button type="button" aria-label={copy.increaseQuantity} disabled={busy || !!pending || (maximum !== undefined && stepQuantity >= maximum)} onClick={() => changeQuantity(stepQuantity + 1)}><Plus aria-hidden="true" width={20} height={20} className="inline-block align-middle shrink-0" /></button>
        </div>
      </div>
      <div className={unavailable ? styles.error : styles.status}>{!unavailable && <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><path d="M3 4h12v13H3zM15 9h4l3 4v4h-7" /><circle cx="7" cy="18" r="2" /><circle cx="18" cy="18" r="2" /></svg>}{unavailable ? copy.stockChanged : variant ? copy.available : copy.stockUnknown}</div>
      </div>
      {variant?.metadata?.demo_price_basis === "model-starting-price" && <p className={styles.muted}>{copy.demo}</p>}
      {error && <p role="alert" className={styles.error} data-testid="product-error-message">{error}</p>}
    </div>
    <div className={styles.lineAside}><span className={styles.price} data-testid="product-price">{money(item.subtotal ?? item.unit_price * item.quantity)}</span><button type="button" className={styles.remove} disabled={busy || !!pending} onClick={remove} data-testid="product-delete-button"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><path d="M4 6h16M9 6V3h6v3M6 6l1 15h10l1-15M10 10v7M14 10v7" /></svg>{pending === "remove" ? copy.removing : copy.remove}</button></div>
  </article>
}
