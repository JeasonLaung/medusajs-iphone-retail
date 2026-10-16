"use client"

import { addToCart } from "@lib/data/cart"
import { getProductPrice, getPricesForVariant } from "@lib/util/get-product-price"
import { getAppleLaunchPrice } from "@lib/util/get-apple-launch-price"
import { useIntersection } from "@lib/hooks/use-in-view"
import { HttpTypes } from "@medusajs/types"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { useParams, usePathname, useSearchParams } from "next/navigation"
import { useEffect, useMemo, useRef, useState } from "react"
import { getProductCopy, optionLabel, optionValueLabel } from "../../i18n"
import { variantOptions as optionMap, variantInStock as available, completeVariants, preferredOptionVariant, purchaseOptions, variantSlug, variantFromLocation } from "@lib/util/product-variants"
import { isModelColor } from "../color-swatches"
import StickyPurchaseBar from "../sticky-purchase-bar"
import styles from "../product-detail/styles.module.css"

export default function ProductActions({ product, disabled, initialVariantId }: {
  product: HttpTypes.StoreProduct
  region: HttpTypes.StoreRegion
  disabled?: boolean
  initialVariantId?: string
}) {
  const { countryCode } = useParams<{ countryCode: string }>()
  const copy = getProductCopy(countryCode)
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const variants = useMemo(() => completeVariants(product), [product])
  const productOptions = purchaseOptions(product)
  const [options, setOptions] = useState<Record<string, string>>(() => {
    const initial = variantFromLocation(variants, pathname, searchParams.get("v_id"), initialVariantId) ?? variants.find(available) ?? variants[0]
    return initial ? optionMap(initial) : {}
  })
  const [isAdding, setIsAdding] = useState(false)
  const [feedback, setFeedback] = useState<"added" | "error" | null>(null)
  const actionsRef = useRef<HTMLDivElement>(null)
  const priceRef = useRef<HTMLDivElement>(null)
  const inView = useIntersection(actionsRef, "0px")
  const selectedVariant = useMemo(() => variants.find((variant) => {
    const values = optionMap(variant)
    return Object.keys(values).length === Object.keys(options).length && Object.entries(values).every(([key, value]) => options[key] === value)
  }), [variants, options])
  const requestedVariantId = variantFromLocation(variants, pathname, searchParams.get("v_id"), initialVariantId)?.id
  useEffect(() => {
    const requested = variants.find((variant) => variant.id === requestedVariantId)
    if (requested) setOptions(optionMap(requested))
    else if (!variants.some((variant) => Object.entries(optionMap(variant)).every(([key, value]) => options[key] === value))) {
      const initial = variants.find(available) ?? variants[0]
      setOptions(initial ? optionMap(initial) : {})
    }
  }, [requestedVariantId, variants])

  const selectOption = (optionId: string, value: string) => {
    const variant = preferredOptionVariant(product, options, optionId, value)
    if (!variant) return
    setOptions(optionMap(variant))
    setFeedback(null)
    const params = new URLSearchParams(searchParams.toString())
    const slug = variantSlug(variant)
    if (slug) params.delete("v_id")
    else params.set("v_id", variant.id)
    const target = slug ? `/${countryCode}/products/${slug}` : pathname
    // Next's native History integration updates the URL without fetching a new
    // server component tree. All variants and their prices are already loaded.
    window.history.replaceState(null, "", target + (params.size ? "?" + params.toString() : "") + window.location.hash)
  }

  useEffect(() => {
    if (!selectedVariant || !variantSlug(selectedVariant)) return
    document.title = `${product.title} ${selectedVariant.title} | Medusa Store`
    const canonical = document.querySelector<HTMLLinkElement>('link[rel="canonical"]')
    if (canonical) canonical.href = new URL(pathname, canonical.href).href
  }, [selectedVariant, pathname, product.title])

  const { cheapestPrice, variantPrice } = getProductPrice({ product, variantId: selectedVariant?.id })
  const price = selectedVariant ? variantPrice : cheapestPrice
  const format = (amount: number, currency: string) => new Intl.NumberFormat(countryCode.endsWith("-zh") ? "zh-CN" : "en-IE", { style: "currency", currency }).format(amount)
  const formattedPrice = price ? format(price.calculated_price_number, price.currency_code) : copy.pricePending
  const inStock = available(selectedVariant)
  const canAdd = inStock && !!selectedVariant && !!price && !disabled && !isAdding
  const buttonText = isAdding ? copy.adding : !selectedVariant ? (Object.keys(options).length ? copy.unavailable : copy.select) : !inStock ? copy.out : !price ? copy.pricePending : copy.add
  const colours = Array.isArray(product.metadata?.card_colors) ? product.metadata.card_colors.filter(isModelColor) : []
  const summary = productOptions.map((option) => options[option.id] && optionValueLabel(options[option.id], countryCode)).filter(Boolean).join(" · ") || selectedVariant?.title
  const headlineSummary = ["storage", "color", "condition"].map((name) => {
    const option = productOptions.find((option) => name === "storage" ? /storage|capacity/i.test(option.title ?? "") : name === "color" ? /colou?r/i.test(option.title ?? "") : /condition/i.test(option.title ?? ""))
    return option && options[option.id] ? optionValueLabel(options[option.id], countryCode) : null
  }).filter(Boolean).join(" | ")
  const launchPrice = getAppleLaunchPrice(product, selectedVariant, price?.currency_code)

  const handleAdd = async () => {
    if (!canAdd || !selectedVariant) return
    setIsAdding(true)
    setFeedback(null)
    try {
      await addToCart({ variantId: selectedVariant.id, quantity: 1, countryCode })
      setFeedback("added")
    } catch {
      setFeedback("error")
    } finally {
      setIsAdding(false)
    }
  }

  const feedbackContent = feedback === "error" ? copy.error : feedback === "added" ? <>{copy.added} · <LocalizedClientLink href="/cart">{copy.viewCart}</LocalizedClientLink></> : null

  return <div className={styles.actions}>
    <div className={styles.productHeading} ref={priceRef}>
      <div><h1 id="product-title" className={styles.title} data-testid="product-title">{product.title}</h1><p className={styles.selectionSummary}>{headlineSummary || copy.select}</p></div>
      <div className={styles.headingPrice}><span className={styles.price} data-testid="product-price" data-value={price?.calculated_price_number}>{!selectedVariant && <span className={styles.muted}>{copy.from} </span>}{formattedPrice}</span>
        {price?.price_type === "sale" && <p className={styles.muted}><s aria-label={copy.original}>{format(price.original_price_number, price.currency_code)}</s></p>}
        <p className={styles.muted} data-testid="apple-launch-price">{copy.appleLaunchPrice}: {launchPrice !== null && price ? format(launchPrice, price.currency_code) : copy.unspecified}</p>
        <p className={styles.muted}>{copy.tax}</p>
      </div>
    </div>
    {selectedVariant?.metadata?.demo_price_basis === "model-starting-price" && <p className={styles.demoPriceNote}>{copy.demoPriceNote}</p>}
    {productOptions.map((option) => <fieldset key={option.id} className={styles.option} disabled={disabled || isAdding}>
      <legend className={styles.legend}>{optionLabel(option.title ?? "", countryCode)}</legend>
      <div className={styles.choices} data-testid="product-options">{Array.from(new Set((option.values ?? []).map((v) => v.value))).map((value) => {
        const candidate = preferredOptionVariant(product, options, option.id, value)
        const inStock = !!candidate
        const checked = options[option.id] === value
        const candidatePrice = candidate ? getPricesForVariant(candidate) : null
        const difference = candidatePrice && price && candidatePrice.currency_code === price.currency_code ? candidatePrice.calculated_price_number - price.calculated_price_number : null
        const optionPrice = !inStock ? copy.out : !candidatePrice ? copy.pricePending : difference === null || difference === 0 || checked ? format(candidatePrice.calculated_price_number, candidatePrice.currency_code) : `${difference > 0 ? "+" : "−"} ${format(Math.abs(difference), candidatePrice.currency_code)}`
        const colour = /colou?r/i.test(option.title ?? "") ? colours.find((colour) => colour.name.toLowerCase() === value.toLowerCase()) : null
        return <label key={value} className={styles.choice} data-checked={checked} data-disabled={!inStock || disabled || isAdding}>
          <input type="radio" name={`${product.id}-${option.id}`} value={value} checked={checked} disabled={!inStock} onChange={() => selectOption(option.id, value)} aria-label={optionValueLabel(value, countryCode)} data-testid="option-radio" />
          <span className={styles.choiceLabel}>{colour && <span className={styles.swatch} style={{ background: colour.hex }} aria-hidden="true" />}{optionValueLabel(value, countryCode)}</span>
          <span className={styles.choicePrice}>{optionPrice}</span>
        </label>
      })}</div>
    </fieldset>)}
    {variants.length < (product.variants?.length ?? 0) && <p className={styles.muted + " mt-4"}>{copy.configurationPending}</p>}
    <div className={styles.purchase} ref={actionsRef}>
      <div className={styles.purchaseTop}><div><strong>{product.title}</strong><p className={styles.muted}>{summary || copy.select}</p></div><span className={styles.purchasePrice}>{formattedPrice}</span></div>
      {selectedVariant?.sku && <p className={styles.muted + " mb-3"}>{copy.sku}: {selectedVariant.sku}</p>}
      {inStock && <p className={styles.stock}>{copy.stock}</p>}
      <button type="button" className={styles.add} disabled={!canAdd} onClick={handleAdd} data-testid="add-product-button">{buttonText}</button>
      <div role="status" aria-live="polite" className={styles.feedback + (feedback === "error" ? " " + styles.error : "")}>{feedbackContent}</div>
    </div>
    <div className={styles.support}>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true"><path d="M3 5h12v12H3zM15 9h4l3 4v4h-7M1 9h7M1 12h5" /><circle cx="7" cy="18" r="2" /><circle cx="18" cy="18" r="2" /></svg>
      <div><strong>{copy.shipping}</strong><p className={styles.muted}>{copy.shippingText}</p><LocalizedClientLink href="/help">{copy.help} ↗</LocalizedClientLink></div>
    </div>
    <StickyPurchaseBar anchor={priceRef} title={product.title ?? ""} summary={summary || copy.select} price={formattedPrice} label={copy.summary} buttonText={buttonText} disabled={!canAdd} onAdd={handleAdd}>
      {feedbackContent && <p className={styles.feedback + (feedback === "error" ? " " + styles.error : "")}>{feedbackContent}</p>}
    </StickyPurchaseBar>
    {!inView && <div className={styles.mobileBar}><div><span className={styles.muted}>{product.title}</span><strong>{formattedPrice}</strong></div><button type="button" className={styles.add} disabled={!canAdd} onClick={handleAdd}>{buttonText}</button></div>}
  </div>
}
