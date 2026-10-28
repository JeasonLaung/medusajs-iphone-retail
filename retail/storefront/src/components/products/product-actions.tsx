import {ArrowUpRightMini} from './icons'
import {productPrice,variantPrice as getPricesForVariant,appleLaunchPrice as getAppleLaunchPrice,optionPriceDifference} from '../../lib/products.ts'
import {formatAmount} from '../../lib/money.ts'
import {useProductSelection,type ProductSelectionProps} from '../../lib/use-product-selection.ts'
import iphoneThumbnails from './iphone-thumbnails.json'
import {useIntersection} from '../../lib/use-in-view'
import Image from './product-image'
import LocalizedClientLink from './localized-link'
import GradeBadge,{isGrade} from './grade-badge'
import SelectionRow from './selection-row'
import {useEffect,useRef,useState} from 'react'
import {getProductCopy,optionLabel,optionValueLabel} from './i18n'
import {variantInStock as available,completeVariants,preferredOptionVariant} from '../../lib/product-variants.ts'
import StickyPurchaseBar from './sticky-purchase-bar'
import GradeGuide from './grade-guide'
import StorefrontTheme from './storefront-theme'
import styles from './styles.module.css'
import {cartInventoryLimitReached,type CartQuantities} from '../../lib/cart-quantities.ts'

export type CartAddResult = {ok:true;quantities:CartQuantities} | {ok:false;error:'inventory'|'error'}
// Required boundary: a caller must explicitly supply a real commerce adapter.
// Existing Gin catalog checkout=false must be passed as enabled:false.
export type ProductCommerce = {enabled:false} | {enabled:true;addToCart:(input:{variantId:string;quantity:'1';countryCode:string})=>Promise<CartAddResult>;navigate?:(path:string)=>void}
export type ProductActionsProps = ProductSelectionProps & {disabled?:boolean;cartError?:boolean;cartQuantities:CartQuantities;commerce:ProductCommerce}

export default function ProductActions(props:ProductActionsProps) {
 return <StorefrontTheme locale={props.segment.endsWith('-zh')?'zh-CN':'en'}><Actions {...props}/></StorefrontTheme>
}

function Actions(props:ProductActionsProps) {
  const {product,disabled,cartError=false,cartQuantities,commerce,segment:countryCode}=props
  const copy=getProductCopy(countryCode)
  const {variant:selectedVariant,options,purchaseOptions:productOptions,selectOption:choose}=useProductSelection(props)
  const variants=completeVariants(product)
  const [isAdding, setIsAdding] = useState(false)
  const [feedback, setFeedback] = useState<"added" | "error" | "inventory" | null>(null)
  const [quantities, setQuantities] = useState(cartQuantities)
  useEffect(() => setQuantities(cartQuantities), [cartQuantities])
  const actionsRef = useRef<HTMLDivElement>(null)
  const priceRef = useRef<HTMLDivElement>(null)
  const inView = useIntersection(actionsRef, "0px")
  const selectOption=(optionId:string,value:string)=>{if(choose(optionId,value))setFeedback(null)}

  const { cheapestPrice, variantPrice } = productPrice(product,selectedVariant?.id)
  const price = selectedVariant ? variantPrice : cheapestPrice
  const format = (amount: string, currency: string) => formatAmount(amount,currency,countryCode.endsWith("-zh") ? "zh-CN" : "en-IE")
  const formattedPrice = price ? format(price.calculated_amount, price.currency_code) : copy.pricePending
  const inStock = available(selectedVariant)
  const atCartLimit = inStock && cartInventoryLimitReached(selectedVariant, quantities)
  const canAdd = commerce.enabled && inStock && !atCartLimit && !!selectedVariant && !!price && !disabled && !isAdding
  const buttonText = isAdding ? copy.adding : !selectedVariant ? (Object.keys(options).length ? copy.unavailable : copy.select) : !inStock ? copy.out : atCartLimit ? copy.cartLimit : !price ? copy.pricePending : copy.add
  const colours = product.display.colors
  const summary = productOptions.map((option) => options[option.id] && optionValueLabel(options[option.id], countryCode)).filter(Boolean).join(" · ") || selectedVariant?.title
  const headlineSummary = ["storage", "color", "condition"].map((name) => {
    const option = productOptions.find((option) => name === "storage" ? /storage|capacity/i.test(option.title ?? "") : name === "color" ? /colou?r/i.test(option.title ?? "") : /condition/i.test(option.title ?? ""))
    return option && options[option.id] ? optionValueLabel(options[option.id], countryCode) : null
  }).filter(Boolean).join(" | ")
  const launchPrice = getAppleLaunchPrice(product, selectedVariant, price?.currency_code)
  const colorOption = productOptions.find((option) => /colou?r/i.test(option.title ?? ""))
  const colorSlug = colorOption ? (options[colorOption.id] ?? "").toLowerCase().replace(/[^a-z0-9]+/g, "-") : ""
  const modelSlug = product.handle?.replace(/^swappie-/, "") ?? ""
  const localThumbnail = (iphoneThumbnails as Record<string, string>)[`${modelSlug}/${colorSlug}`]
  const thumbnail = selectedVariant?.thumbnail ?? selectedVariant?.images?.find((image) => image.url)?.url ?? localThumbnail ?? product.thumbnail ?? product.images?.find((image) => image.url)?.url

  const handleAdd = async () => {
    if (!canAdd || !selectedVariant || !commerce.enabled) return
    setIsAdding(true)
    setFeedback(null)
    try {
      const result = await commerce.addToCart({variantId:selectedVariant.id,quantity:'1',countryCode})
      if (result.ok) {
        setQuantities(result.quantities)
        setFeedback("added")
        if (commerce.navigate) commerce.navigate(`/${countryCode}/cart`); else window.location.assign(`/${countryCode}/cart`)
      } else {
        setFeedback(result.error)
      }
    } catch {
      setFeedback("error")
    } finally {
      setIsAdding(false)
    }
  }

  const feedbackContent = feedback === "error" ? copy.error : feedback === "inventory" ? <>{copy.inventoryError} · <LocalizedClientLink segment={countryCode} href="/cart">{copy.viewCart}</LocalizedClientLink></> : atCartLimit ? <>{copy.cartLimitMessage} · <LocalizedClientLink segment={countryCode} href="/cart">{copy.viewCart}</LocalizedClientLink></> : feedback === "added" ? <>{copy.added} · <LocalizedClientLink segment={countryCode} href="/cart">{copy.viewCart}</LocalizedClientLink></> : cartError ? copy.error : null

  return <div className={styles.actions}>
    <div className={styles.productHeading} ref={priceRef}>
      <div><h1 id="product-title" className={styles.title} data-testid="product-title">{product.title}</h1><p className={styles.selectionSummary}>{headlineSummary || copy.select}</p></div>
      <div className={styles.headingPrice}><span className={styles.price} data-testid="product-price" data-value={price?.calculated_amount}>{!selectedVariant && <span className={styles.muted}>{copy.from} </span>}{formattedPrice}</span>
        {price?.price_type === "sale" && <p className={styles.muted}><s aria-label={copy.original}>{format(price.original_amount, price.currency_code)}</s></p>}
        <p className={styles.muted} data-testid="apple-launch-price">{copy.appleLaunchPrice}: {launchPrice !== null && price ? format(launchPrice, price.currency_code) : copy.unspecified}</p>
        <p className={styles.muted}>{copy.tax}</p>
      </div>
    </div>
    {selectedVariant?.display.demo_price_basis === "model-starting-price" && <p className={styles.demoPriceNote}>{copy.demoPriceNote}</p>}
    {productOptions.map((option) => <fieldset key={option.id} className={styles.option} disabled={disabled || isAdding}>
      <legend className={styles.legend}>{optionLabel(option.title ?? "", countryCode)}{/condition|grade/i.test(option.title ?? "") && option.values?.some(({ value }) => value === "S" || value === "A") && <GradeGuide locale={countryCode} selectedGrade={options[option.id]} />}</legend>
      <div className={styles.choices} data-testid="product-options">{Array.from(new Set((option.values ?? []).map((v) => v.value))).map((value) => {
        const candidate = preferredOptionVariant(product, options, option.id, value)
        const inStock = !!candidate
        const checked = options[option.id] === value
        const candidatePrice = candidate ? getPricesForVariant(candidate) : null
        const difference = candidate && selectedVariant ? optionPriceDifference(candidate,selectedVariant) : null
        const optionPrice = !inStock ? copy.out : !candidatePrice ? copy.pricePending : difference === null || difference.sign === 0 || checked ? format(candidatePrice.calculated_amount, candidatePrice.currency_code) : `${difference.sign > 0 ? "+" : "−"} ${format(difference.amount, candidatePrice.currency_code)}`
        const colour = /colou?r/i.test(option.title ?? "") ? colours.find((colour) => colour.name.toLowerCase() === value.toLowerCase()) : null
        return <SelectionRow key={value} name={`${product.id}-${option.id}`} value={value} checked={checked} disabled={!inStock || disabled || isAdding} onChange={() => selectOption(option.id, value)} aria-label={optionValueLabel(value, countryCode)} data-testid="option-radio" trailing={!checked ? optionPrice : undefined}>
          {colour && <span className={styles.swatch} style={{ background: colour.hex }} aria-hidden="true" />}{/condition|grade/i.test(option.title ?? "") && isGrade(value) ? <GradeBadge grade={value} locale={countryCode} /> : optionValueLabel(value, countryCode)}
        </SelectionRow>
      })}</div>
    </fieldset>)}
    {variants.length < (product.variants?.length ?? 0) && <p className={styles.muted + " " + styles.mt4}>{copy.configurationPending}</p>}
    <div className={styles.purchase} ref={actionsRef}>
      <div className={styles.purchaseTop}><div><strong>{product.title}</strong><p className={styles.muted}>{summary || copy.select}</p></div><span className={styles.purchasePrice}>{formattedPrice}</span></div>
      {inStock && <p className={styles.stock}>{copy.stock}</p>}
      <button type="button" className={styles.add} disabled={!canAdd} onClick={handleAdd} data-testid="add-product-button">{buttonText}</button>
      <div role="status" aria-live="polite" className={styles.feedback + (feedback === "error" ? " " + styles.error : "")}>{feedbackContent}</div>
    </div>
    <div className={styles.support}>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true"><path d="M3 5h12v12H3zM15 9h4l3 4v4h-7M1 9h7M1 12h5" /><circle cx="7" cy="18" r="2" /><circle cx="18" cy="18" r="2" /></svg>
      <div><strong>{copy.shipping}</strong><p className={styles.muted}>{copy.shippingText}</p><LocalizedClientLink segment={countryCode} href="/help">{copy.help} <ArrowUpRightMini aria-hidden="true" width={16} height={16} className={styles.icon} /></LocalizedClientLink></div>
    </div>
    <StickyPurchaseBar anchor={priceRef} title={product.title ?? ""} summary={summary || copy.select} price={formattedPrice} label={copy.summary} buttonText={buttonText} disabled={!canAdd} onAdd={handleAdd}>
      {feedbackContent && <p className={styles.feedback + (feedback === "error" ? " " + styles.error : "")}>{feedbackContent}</p>}
    </StickyPurchaseBar>
    {!inView && <aside className={styles.mobileBar} aria-label={copy.summary} data-testid="mobile-purchase-bar">
      <div className={styles.mobileBarSelection}>
        {thumbnail && <div className={styles.mobileBarThumbnail}><Image src={thumbnail} alt="" fill sizes="48px" /></div>}
        <div className={styles.mobileBarDetails}>
          <span className={styles.muted}>{product.title}</span>
          <p className={styles.mobileBarSummary} data-testid="mobile-purchase-summary">{summary || copy.select}</p>
        </div>
      </div>
      <strong>{formattedPrice}</strong>
      <button type="button" className={styles.add} disabled={!canAdd} onClick={handleAdd}>{buttonText}</button>
    </aside>}
  </div>
}
