import {publicJSON,PUBLIC_NOT_FOUND} from './gin-public.ts'
import {amountParts,compareAmounts,differenceAmounts,formatAmount,percentageDecrease} from './money.ts'
import {projectProductDisplay,projectVariantDisplay,type ProductDisplay,type VariantDisplay} from './product-display.ts'

export type Image = {id: string; url: string; rank: number}
export type OptionValue = {id: string; value: string; rank: number | null}
export type Option = {id: string; title: string; values: OptionValue[]}
export type Quote = {
 calculated_amount: string; original_amount: string | null; currency_code: string
 is_calculated_price_tax_inclusive: boolean; is_original_price_tax_inclusive: boolean
 is_calculated_price_price_list: boolean; is_original_price_price_list: boolean
 calculated_price: {price_list_type: 'sale' | 'override' | null}
}
export type Variant = {
 id: string; title: string; sku: string | null; thumbnail: string | null; variant_rank: number | null
 options: {id: string; option_id: string; value: string}[]; images: Image[]
 configuration_complete: boolean; calculated_price: Quote | null
 manage_inventory: boolean; allow_backorder: boolean; inventory_quantity: string | null
 availability: {state: 'available' | 'out_of_stock' | 'backorder' | 'unavailable' | 'incomplete' | 'untracked'; can_fulfill: boolean; max_fulfillable_quantity: string | null}
 display: VariantDisplay
}
export type Product = {
 id: string; title: string; handle: string; subtitle: string | null; description: string | null
 thumbnail: string | null; collection_id: string | null; options: Option[]; images: Image[]; variants: Variant[]
 display: ProductDisplay
}
export type ProductQuery = {limit?: number; offset?: number; q?: string; handle?: string; category_id?: string; collection_id?: string; region_id?: string; quantity?: string; order?: '-created_at'; option_value_id?: string[]}
export type PriceContext = Pick<ProductQuery,'region_id' | 'quantity'>
export type ProductPage = {products: Product[]; count: number; limit: number; offset: number; capabilities: {pricing: boolean; availability: true; checkout: false}}
const unavailable = () => new Error('Store products unavailable')
function object(v: unknown): Record<string,unknown> { if (!v || typeof v !== 'object' || Array.isArray(v)) throw unavailable(); return v as Record<string,unknown> }
function text(v: unknown): string { if (typeof v !== 'string') throw unavailable(); return v }
function id(v: unknown): string { const s = text(v); if (!s || Array.from(s).length > 191 || /[\x00-\x1f\x7f]/.test(s)) throw unavailable(); return s }
function nullableText(v: unknown): string | null { return v === null ? null : text(v) }
function bool(v: unknown): boolean { if (typeof v !== 'boolean') throw unavailable(); return v }
function integer(v: unknown): number { if (typeof v !== 'number' || !Number.isSafeInteger(v)) throw unavailable(); return v }
function rank(v: unknown): number | null { return v === null ? null : integer(v) }
function list(v: unknown, max: number): unknown[] { if (!Array.isArray(v) || v.length > max) throw unavailable(); return v }
function unique(values: string[]): void { if (new Set(values).size !== values.length) throw unavailable() }
function quantity(v: unknown): string | null { if (v === null) return null; const s = text(v); if (s.length > 128 || !/^(0|[1-9][0-9]*)$/.test(s)) throw unavailable(); return s }
function amount(v: unknown): string { const s = text(v); amountParts(s); return s }
function image(input: unknown): Image { const v = object(input); return {id:id(v.id),url:text(v.url),rank:integer(v.rank)} }
function quote(input: unknown): Quote | null {
 if (input === null) return null
 const v = object(input), kind = object(v.calculated_price).price_list_type
 if (kind !== null && kind !== 'sale' && kind !== 'override' || !/^[a-z]{3}$/.test(text(v.currency_code))) throw unavailable()
 const out: Quote = {calculated_amount:amount(v.calculated_amount),original_amount:v.original_amount === null ? null : amount(v.original_amount),currency_code:text(v.currency_code),is_calculated_price_tax_inclusive:bool(v.is_calculated_price_tax_inclusive),is_original_price_tax_inclusive:bool(v.is_original_price_tax_inclusive),is_calculated_price_price_list:bool(v.is_calculated_price_price_list),is_original_price_price_list:bool(v.is_original_price_price_list),calculated_price:{price_list_type:kind}}
 if (out.is_calculated_price_price_list !== (kind !== null)) throw unavailable()
 return out
}
function availability(input: unknown, v: Pick<Variant,'configuration_complete' | 'manage_inventory' | 'allow_backorder' | 'inventory_quantity'>, requested: bigint): Variant['availability'] {
 const data = object(input), state = text(data.state), can = bool(data.can_fulfill), max = quantity(data.max_fulfillable_quantity), pooled = v.inventory_quantity
 if ((max === null) !== (pooled === null) || max !== null && pooled !== null && BigInt(max) > BigInt(pooled)) throw unavailable()
 if (!v.configuration_complete) {
  if (state !== 'incomplete' || can || max !== null) throw unavailable()
 } else if (!v.manage_inventory) {
  if (state !== 'untracked' || !can || max !== null) throw unavailable()
 } else if (state === 'unavailable') {
  if (can || max !== null && max !== '0') throw unavailable()
 } else {
  if (max === null) throw unavailable()
  const enough = BigInt(max) >= requested
  if (state === 'available' ? !can || !enough : state === 'backorder' ? !can || enough || !v.allow_backorder : state === 'out_of_stock' ? can || enough || v.allow_backorder : true) throw unavailable()
 }
 return {state:state as Variant['availability']['state'],can_fulfill:can,max_fulfillable_quantity:max}
}
function product(input: unknown, pricing: boolean, requested: bigint, consume: (n: number) => void): Product {
 const p = object(input)
 const options = list(p.options,1000).map(input => {
  const o = object(input), values = list(o.values,10000).map(input => { const v = object(input); return {id:id(v.id),value:text(v.value),rank:rank(v.rank)} })
  unique(values.map(v=>v.id)); consume(values.length)
  return {id:id(o.id),title:text(o.title),values}
 })
 unique(options.map(o=>o.id))
 const images = list(p.images,1000).map(image); unique(images.map(im=>im.id))
 const variants = list(p.variants,1000).map(input => {
  const v = object(input), choices = list(v.options,1000).map(input => { const o = object(input); return {id:id(o.id),option_id:id(o.option_id),value:text(o.value)} })
  unique(choices.map(o=>o.id))
  for (const choice of choices) if (!options.some(o=>o.id === choice.option_id && o.values.some(value=>value.id === choice.id && value.value === choice.value))) throw unavailable()
  const complete = bool(v.configuration_complete)
  // A rejected duplicate/foreign source choice can leave a full-looking public
  // subset, so only the server may declare an otherwise valid subset incomplete.
  if (complete && (choices.length !== options.length || new Set(choices.map(o=>o.option_id)).size !== choices.length)) throw unavailable()
  const variantImages = list(v.images,1000).map(image); unique(variantImages.map(im=>im.id))
  for (const im of variantImages) if (!images.some(parent=>parent.id === im.id && parent.url === im.url && parent.rank === im.rank)) throw unavailable()
  const price = quote(v.calculated_price)
  if (price !== null && (!pricing || !complete)) throw unavailable()
  const out = {id:id(v.id),title:text(v.title),sku:nullableText(v.sku),thumbnail:nullableText(v.thumbnail),variant_rank:rank(v.variant_rank),options:choices,images:variantImages,configuration_complete:complete,calculated_price:price,manage_inventory:bool(v.manage_inventory),allow_backorder:bool(v.allow_backorder),inventory_quantity:quantity(v.inventory_quantity)}
  consume(choices.length + variantImages.length)
  return {...out,display:projectVariantDisplay(v.display),availability:availability(v.availability,out,requested)}
 })
 unique(variants.map(v=>v.id)); consume(options.length + images.length + variants.length)
 return {id:id(p.id),title:text(p.title),handle:text(p.handle),subtitle:nullableText(p.subtitle),description:nullableText(p.description),thumbnail:nullableText(p.thumbnail),collection_id:p.collection_id === null ? null : id(p.collection_id),options,images,variants,display:projectProductDisplay(p.display)}
}
function capabilities(input: unknown, pricing: boolean): ProductPage['capabilities'] {
 const v = object(input)
 if (v.pricing !== pricing || v.availability !== true || v.checkout !== false) throw unavailable()
 return {pricing,availability:true,checkout:false}
}
function query(input: ProductQuery, detail = false): {params: URLSearchParams; limit: number; offset: number; pricing: boolean; requested: bigint} {
 const q = object(input), params = new URLSearchParams()
 const allowed = detail ? ['region_id','quantity'] : ['limit','offset','q','handle','category_id','collection_id','region_id','quantity','order','option_value_id']
 for (const [key,value] of Object.entries(q)) {
  if (!allowed.includes(key)) throw unavailable()
  if (value === undefined) continue
  if (key === 'option_value_id') {
   const values = list(value,100).map(id)
   if (!values.length) throw unavailable()
   for (const v of new Set(values)) params.append(key,v)
  } else if (key === 'order') {
   if (value !== '-created_at') throw unavailable()
   params.set(key,value)
  } else if (key === 'limit' || key === 'offset') {
   const n = integer(value)
   if (key === 'limit' ? n < 1 || n > 100 : n < 0 || n > 100000) throw unavailable()
   params.set(key,String(n))
  } else if (key === 'quantity') {
   const s = quantity(value)
   if (s === null || s === '0' || BigInt(s) > 1000000000n || q.region_id === undefined) throw unavailable()
   params.set(key,s)
  } else {
   const s = text(value)
   if (key === 'q' || key === 'handle') { if (!s || Array.from(s).length > 512 || /[\x00-\x1f\x7f]/.test(s)) throw unavailable() } else id(s)
   params.set(key,s)
  }
 }
 return {params,limit:q.limit === undefined ? 12 : integer(q.limit),offset:q.offset === undefined ? 0 : integer(q.offset),pricing:params.has('region_id'),requested:BigInt(params.get('quantity') ?? '1')}
}
function projection(pricing: boolean, requested: bigint): (input: unknown) => Product {
 let budget = 10000
 return input => product(input,pricing,requested,n => { budget -= n; if (budget < 0) throw unavailable() })
}
export async function loadProducts(origin: string | undefined, input: ProductQuery = {}, fetcher: typeof fetch = fetch): Promise<ProductPage> {
 try {
  const q = query(input), body = object(await publicJSON(origin,`/api/store/products?${q.params}`,8*1024*1024,fetcher)), count = integer(body.count)
  if (count < 0 || body.limit !== q.limit || body.offset !== q.offset) throw unavailable()
  const products = list(body.products,q.limit).map(projection(q.pricing,q.requested))
  if (products.length !== Math.min(q.limit,Math.max(count-q.offset,0))) throw unavailable()
  unique(products.map(p=>p.id)); unique(products.flatMap(p=>p.variants.map(v=>v.id)))
  return {products,count,limit:q.limit,offset:q.offset,capabilities:capabilities(body.capabilities,q.pricing)}
 } catch { throw unavailable() }
}
export async function loadProduct(origin: string | undefined, productID: string, context: PriceContext = {}, fetcher: typeof fetch = fetch): Promise<Product | null> {
 try {
  id(productID)
  const q = query(context,true), raw = await publicJSON(origin,`/api/store/products/${encodeURIComponent(productID)}?${q.params}`,8*1024*1024,fetcher,true)
  if (raw === PUBLIC_NOT_FOUND) return null
  const body = object(raw); capabilities(body.capabilities,q.pricing)
  const p = projection(q.pricing,q.requested)(body.product)
  if (p.id !== productID) throw unavailable()
  return p
 } catch { throw unavailable() }
}

export function variantPrice(v: Variant | undefined, locale = 'en-US') {
 const p = v?.calculated_price
 if (!v?.configuration_complete || !p) return null
 const original = p.original_amount ?? p.calculated_amount
 return {calculated_amount:p.calculated_amount,original_amount:original,calculated_price:formatAmount(p.calculated_amount,p.currency_code,locale),original_price:formatAmount(original,p.currency_code,locale),currency_code:p.currency_code,price_type:p.calculated_price.price_list_type ?? 'default',percentage_diff:percentageDecrease(original,p.calculated_amount)}
}
export function productPrice(product: Product, variantID?: string, locale = 'en-US') {
 let cheapest: Variant | undefined
 const currencies = new Set(product.variants.flatMap(v=>v.calculated_price ? [v.calculated_price.currency_code] : []))
 if (currencies.size > 1) throw unavailable()
 for (const v of product.variants) if (v.configuration_complete && v.calculated_price && (!cheapest || compareAmounts(v.calculated_price.calculated_amount,cheapest.calculated_price!.calculated_amount) < 0)) cheapest = v
 return {cheapestPrice:variantPrice(cheapest,locale),variantPrice:variantPrice(product.variants.find(v=>v.id === variantID || v.sku === variantID),locale)}
}
export function optionPriceDifference(candidate: Variant, selected: Variant): {sign: -1 | 0 | 1; amount: string} | null {
 const a = candidate.configuration_complete && candidate.calculated_price, b = selected.configuration_complete && selected.calculated_price
 return a && b && a.currency_code === b.currency_code ? differenceAmounts(a.calculated_amount,b.calculated_amount) : null
}

// Original EUR reference values and precedence from the existing storefront's
// get-apple-launch-price.ts; this is historical copy, not live exchange pricing.
export function appleLaunchPrice(product: Product, variant?: Variant, currency?: string): string | null {
 const option = product.options.find(o=>/storage|capacity/i.test(o.title))
 const storage = variant?.options.find(o=>o.option_id === option?.id)?.value
 if (!storage || !currency) return null
 const raw = variant?.display.apple_launch_price ?? product.display.launch_prices.find(p=>p.storage === storage)
 if (raw && raw.currency_code === currency) return raw.amount
 const references: Record<string,Record<string,string>> = {'iphone 13':{'128gb':'869','256gb':'979','512gb':'1189'}}
 return currency === 'eur' ? references[product.title.toLowerCase()]?.[storage.replace(/\s/g,'').toLowerCase()] ?? null : null
}
