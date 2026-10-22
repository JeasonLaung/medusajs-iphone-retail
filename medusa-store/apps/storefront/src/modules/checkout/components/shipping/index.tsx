"use client"

import { setShippingMethod } from "@lib/data/cart"
import { startMolliePayment } from "@lib/data/mollie"
import { calculatePriceForShippingOption } from "@lib/data/fulfillment"
import { HttpTypes } from "@medusajs/types"
import { useParams } from "next/navigation"
import { useEffect, useState } from "react"
import { Button } from "@modules/common/components/ui"
import { getCheckoutCopy } from "../../i18n"
import styles from "../../styles.module.css"

type Option = HttpTypes.StoreCartShippingOption
const isPickup = (option: Option) => (option as unknown as { service_zone?: { fulfillment_set?: { type?: string } } }).service_zone?.fulfillment_set?.type === "pickup"

export default function Shipping({ cart, availableShippingMethods }: { cart: HttpTypes.StoreCart; availableShippingMethods: Option[] | null }) {
  const { countryCode } = useParams<{ countryCode: string }>()
  const copy = getCheckoutCopy(countryCode)
  const [selected, setSelected] = useState(cart.shipping_methods?.at(-1)?.shipping_option_id ?? "")
  const [saving, setSaving] = useState(false)
  const [paying, setPaying] = useState(false)
  const [loading, setLoading] = useState(true)
  const [prices, setPrices] = useState<Record<string, number>>({})
  const [error, setError] = useState<string | null>(null)
  const methods = availableShippingMethods ?? []
  const money = (amount: number) => new Intl.NumberFormat(countryCode.endsWith("-zh") ? "zh-CN" : "en-IE", { style: "currency", currency: cart.currency_code }).format(amount)

  useEffect(() => {
    let active = true
    setLoading(true)
    Promise.allSettled((availableShippingMethods ?? []).filter((method) => method.price_type === "calculated").map((method) => calculatePriceForShippingOption(method.id, cart.id))).then((results) => {
      if (!active) return
      const calculated: Record<string, number> = {}
      results.forEach((result) => {
        if (result.status === "fulfilled" && result.value?.id && typeof result.value.amount === "number") calculated[result.value.id] = result.value.amount
      })
      setPrices(calculated)
      setLoading(false)
    })
    return () => { active = false }
  }, [availableShippingMethods, cart.id])

  const choose = async (id: string) => {
    if (saving || paying) return
    setSaving(true)
    setError(null)
    const previous = selected
    setSelected(id)
    try { await setShippingMethod({ cartId: cart.id, shippingMethodId: id }) }
    catch { setSelected(previous); setError(copy.selectDelivery) }
    finally { setSaving(false) }
  }
  return <section aria-labelledby="shipping-heading">
    <h2 id="shipping-heading">{copy.shipping}</h2><p className={styles.muted}>{copy.deliveryHelp}</p>
    {!methods.length && <p role="alert" className={styles.error}>{copy.noDelivery}</p>}
    {[false, true].map((pickup) => {
      const options = methods.filter((method) => isPickup(method) === pickup)
      if (!options.length) return null
      return <div key={String(pickup)}><h3 className={styles.sectionLabel}>{pickup ? copy.pickup : copy.homeDelivery}</h3>
        {options.map((method) => {
          const amount = method.price_type === "flat" ? method.amount : prices[method.id]
          const unavailable = saving || paying || method.insufficient_inventory || typeof amount !== "number"
          return <label key={method.id} className={styles.method} data-checked={selected === method.id} data-disabled={!!unavailable}>
            <input type="radio" name="delivery-method" value={method.id} checked={selected === method.id} disabled={!!unavailable} onChange={() => choose(method.id)} data-testid="delivery-option-radio" />
            <div><strong>{method.name}</strong>{method.type?.description && <p className={styles.muted}>{method.type.description}</p>}</div><strong className={styles.methodPrice}>{typeof amount === "number" ? money(amount) : loading ? "…" : copy.pricePending}</strong>
          </label>
        })}
      </div>
    })}
    {error && <p role="alert" className={styles.error}>{error}</p>}
    <Button className={styles.primary} onClick={async () => {
      if (saving || paying) return
      setPaying(true)
      setError(null)
      try { window.location.assign(await startMolliePayment()) }
      catch (err) {
        setError(err instanceof Error ? err.message : copy.paymentError)
        setPaying(false)
      }
    }} disabled={saving || paying || !selected || !cart.shipping_methods?.some((method) => method.shipping_option_id === selected) || !methods.some((method) => method.id === selected && !method.insufficient_inventory && typeof (method.price_type === "flat" ? method.amount : prices[method.id]) === "number")} isLoading={saving || paying} data-testid="submit-delivery-option-button">{copy.continuePayment}</Button>
  </section>
}
