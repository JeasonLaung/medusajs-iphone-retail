"use client"

import { useState } from "react"
import { useParams } from "next/navigation"
import { applyPromotions } from "@lib/data/cart"
import { HttpTypes } from "@medusajs/types"
import { getCheckoutCopy } from "../../i18n"
import styles from "../../styles.module.css"

export default function DiscountCode({ cart }: { cart: HttpTypes.StoreCart }) {
  const { countryCode } = useParams<{ countryCode: string }>()
  const copy = getCheckoutCopy(countryCode)
  const [code, setCode] = useState("")
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const promotions = cart.promotions ?? []
  const update = async (codes: string[]) => {
    if (pending) return
    setPending(true)
    setError(null)
    try {
      const updated = await applyPromotions(codes)
      if (codes.some((entry) => !updated.promotions?.some((promotion) => promotion.code?.toLowerCase() === entry.toLowerCase()))) setError(copy.promoInvalid)
      else setCode("")
    }
    catch { setError(copy.promoError) }
    finally { setPending(false) }
  }
  return <div className={styles.promo}>
    <form className={styles.promoRow} onSubmit={(event) => {
      event.preventDefault()
      if (code.trim()) update(Array.from(new Set([...promotions.map((promotion) => promotion.code).filter((entry): entry is string => !!entry), code.trim()])))
    }}>
      <label className="sr-only" htmlFor="checkout-discount">{copy.promotion}</label>
      <input id="checkout-discount" name="code" placeholder={copy.promotionPlaceholder} value={code} onChange={(event) => setCode(event.target.value)} disabled={pending} data-testid="discount-input" />
      <button type="submit" disabled={pending || !code.trim()} data-testid="discount-apply-button">{pending ? "…" : copy.apply}</button>
    </form>
    {error && <p role="alert" className={styles.error} data-testid="discount-error-message">{error}</p>}
    {promotions.map((promotion) => <div key={promotion.id} className={styles.promoTag}><span>{promotion.code}</span>{!promotion.is_automatic && <button type="button" disabled={pending} aria-label={copy.removePromotion} onClick={() => update(promotions.filter((entry) => entry.id !== promotion.id).map((entry) => entry.code).filter((entry): entry is string => !!entry))}>{copy.removePromotion}</button>}</div>)}
  </div>
}
