// Browser private transport for Gin. It deliberately shares neither the public
// server fetcher (which omits cookies) nor Medusa's SDK/authentication headers.
export type CartSnapshot = {
 id: string; region_id: string; sales_channel_id: string; currency_code: string
 locale: string; version: string
}
export type CartScope = {csrfToken: string}
export type CartCreation = {operationID: string; regionID: string; locale?: string}
export type CartCreated = {cart: CartSnapshot; csrfToken: string; expiresAt: string; replayed: boolean}
export type CartItemAddition = {operationID:string; version:string; variantID:string; quantity:string}
export type CartItemReceipt = {cart_id:string; version:string; line_id:string; quantity:string; replayed:boolean}
export type CartAmounts = {
 cart_id: string; version: string; currency_code: string; totals: Record<string,string>
 lines: {id:string;variant_id:string|null;unit_price:string;quantity:string;is_tax_inclusive:boolean;totals:Record<string,string>}[]
 shipping_methods: {id:string;name:string;amount:string;is_tax_inclusive:boolean;totals:Record<string,string>}[]
 pricing_recalculated: false; checkout: false
}
export type CartErrorKind = 'input' | 'session' | 'csrf' | 'conflict' | 'region' | 'item' | 'rate_limit' | 'unavailable' | 'protocol'

export class CartError extends Error {
 readonly kind: CartErrorKind
 readonly status: number | null
 // A failed POST may already have committed. Keep its operation ID and scope;
 // callers must not silently create a new operation or rotate creation scope.
 readonly uncertain: boolean
 readonly requestID: string | null
 readonly retryAfter: number | null
 constructor(kind: CartErrorKind, status: number | null = null, uncertain = false, requestID: string | null = null, retryAfter: number | null = null) {
  super(`Cart request ${kind}`)
  this.name = 'CartError'; this.kind = kind; this.status = status; this.uncertain = uncertain
  this.requestID = requestID; this.retryAfter = retryAfter
 }
}

const idPattern = /^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/
const tokenPattern = /^[0-9a-f]{64}$/
const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const versionPattern = /^[1-9][0-9]{0,19}$/
const localePattern = /^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$/
const amountPattern = /^-?(0|[1-9][0-9]{0,191})(\.[0-9]{1,128})?$/
const rowTotals = ['subtotal','total','tax_total','original_subtotal','original_total','original_tax_total','discount_subtotal','discount_tax_total','discount_total']
const cartTotals = [...rowTotals,'item_total','item_subtotal','item_tax_total','item_discount_total','original_item_total','original_item_subtotal','original_item_tax_total','shipping_total','shipping_subtotal','shipping_tax_total','shipping_discount_total','original_shipping_total','original_shipping_subtotal','original_shipping_tax_total','credit_line_total','credit_line_subtotal','credit_line_tax_total']
const protocol = () => new CartError('protocol')
// JavaScript $ also matches before a final newline; API strings are canonical.
function matches(value: unknown, pattern: RegExp): value is string {
 return typeof value === 'string' && !/[\r\n]/.test(value) && pattern.test(value)
}
function record(value: unknown): Record<string,unknown> {
 if (!value || typeof value !== 'object' || Array.isArray(value)) throw protocol()
 return value as Record<string,unknown>
}
function keys(value: Record<string,unknown>, required: string[], optional: string[] = []) {
 if (required.some(key => !Object.hasOwn(value,key)) || Object.keys(value).some(key => !required.includes(key) && !optional.includes(key))) throw protocol()
}
function text(value: unknown, pattern: RegExp): string {
 if (!matches(value,pattern)) throw protocol()
 return value
}
function expiry(value: unknown): string {
 if (!matches(value,/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/) || !Number.isFinite(Date.parse(value))) throw protocol()
 return value
}
function snapshot(value: unknown): CartSnapshot {
 const v = record(value)
 keys(v,['id','region_id','sales_channel_id','currency_code','locale','version'])
 const version = text(v.version,versionPattern)
 if (BigInt(version) > 18446744073709551615n || typeof v.locale !== 'string' || v.locale !== '' && (v.locale.length > 35 || !matches(v.locale,localePattern))) throw protocol()
 return {id:text(v.id,idPattern),region_id:text(v.region_id,idPattern),sales_channel_id:text(v.sales_channel_id,idPattern),currency_code:text(v.currency_code,/^[a-z]{3}$/),locale:v.locale,version}
}
function capabilities(value: unknown) {
 const v = record(value)
 keys(v,['items','checkout'])
 // A draft mutation/amount snapshot does not make the complete page or
 // checkout ready. Never enable purchasing from an unknown capability flag.
 if (v.items !== false || v.checkout !== false) throw protocol()
}

function amount(value: unknown, positive = false): string {
 const v = text(value,amountPattern)
 if (positive && v.startsWith('-')) throw protocol()
 return v
}
function amountMap(value: unknown, fields: string[]): Record<string,string> {
 const v = record(value); keys(v,fields)
 return Object.fromEntries(fields.map(key => [key,amount(v[key])]))
}
function amounts(value: unknown, id: string): CartAmounts {
 const v = record(value)
 keys(v,['cart_id','version','currency_code','totals','lines','shipping_methods','pricing_recalculated','checkout'])
 const version = text(v.version,versionPattern)
 if (v.cart_id !== id || BigInt(version) > 18446744073709551615n || v.pricing_recalculated !== false || v.checkout !== false || !Array.isArray(v.lines) || v.lines.length > 1000 || !Array.isArray(v.shipping_methods) || v.shipping_methods.length > 100) throw protocol()
 const lineIDs = new Set<string>(), shippingIDs = new Set<string>()
 const lines = v.lines.map(value => {
  const row = record(value); keys(row,['id','variant_id','unit_price','quantity','is_tax_inclusive','totals'])
  const id = text(row.id,idPattern), quantity = text(row.quantity,/^[1-9][0-9]{0,9}$/)
  if (lineIDs.has(id) || BigInt(quantity) > 1000000000n || typeof row.is_tax_inclusive !== 'boolean') throw protocol()
  lineIDs.add(id)
  return {id,variant_id:row.variant_id === null ? null : text(row.variant_id,idPattern),unit_price:amount(row.unit_price,true),quantity,is_tax_inclusive:row.is_tax_inclusive,totals:amountMap(row.totals,rowTotals)}
 })
 const shipping = v.shipping_methods.map(value => {
  const row = record(value); keys(row,['id','name','amount','is_tax_inclusive','totals'])
  const id = text(row.id,idPattern)
  if (shippingIDs.has(id) || typeof row.name !== 'string' || row.name.length > 10000 || typeof row.is_tax_inclusive !== 'boolean') throw protocol()
  shippingIDs.add(id)
  return {id,name:row.name,amount:amount(row.amount,true),is_tax_inclusive:row.is_tax_inclusive,totals:amountMap(row.totals,rowTotals)}
 })
 return {cart_id:id,version,currency_code:text(v.currency_code,/^[a-z]{3}$/),totals:amountMap(v.totals,cartTotals),lines,shipping_methods:shipping,pricing_recalculated:false,checkout:false}
}

// Reads only the non-HttpOnly CSRF cookie. Never extracts/forwards the bearer
// possession/scope cookie, stores credentials in localStorage or accepts an
// alternate backend origin. Duplicate cookie names are ambiguous and rejected.
function csrfCookie(cookieHeader: string, secure: boolean, realm:'scope'|'possession'): string | undefined {
 const name = `${secure ? '__Host-' : ''}retail_cart_${realm === 'scope' ? 'scope_csrf' : 'csrf'}`
 const values = cookieHeader.split(';').map(v => v.trim()).filter(v => v.startsWith(name+'=')).map(v => v.slice(name.length+1))
 if (values.length === 0) return undefined
 if (values.length !== 1 || !matches(values[0],tokenPattern)) throw new CartError('session')
 return values[0]
}

export function scopeCSRFFromCookies(cookieHeader: string, secure: boolean): string | undefined {
 return csrfCookie(cookieHeader,secure,'scope')
}
export function cartCSRFFromCookies(cookieHeader: string, secure: boolean): string | undefined {
 return csrfCookie(cookieHeader,secure,'possession')
}

export class CartClient {
 private readonly fetcher: typeof fetch
 private readonly readScopeCSRF: () => string | undefined
 constructor(options: {fetcher: typeof fetch; readScopeCSRF: () => string | undefined}) {
  this.fetcher = options.fetcher; this.readScopeCSRF = options.readScopeCSRF
 }

 private async request(path: string, method: 'GET' | 'POST', body?: string, headers: Record<string,string> = {}, maxBytes = 65536, unprocessable:'region'|'item'='region'): Promise<{status: number; value: unknown}> {
  const controller = new AbortController(), timer = setTimeout(() => controller.abort(),12000)
  const write = method === 'POST'
  try {
   const response = await this.fetcher(path,{method,credentials:'same-origin',cache:'no-store',redirect:'error',signal:controller.signal,headers:{Accept:'application/json',...(write ? {'Content-Type':'application/json'} : {}),...headers},...(body === undefined ? {} : {body})})
   const status = response.status
   if (status !== 200 && status !== 201) {
    await response.body?.cancel().catch(()=>{})
    const kind: CartErrorKind = status === 401 || status === 404 ? 'session' : status === 403 ? 'csrf' : status === 409 ? 'conflict' : status === 422 ? unprocessable : status === 429 ? 'rate_limit' : status === 400 || status === 413 || status === 415 ? 'input' : 'unavailable'
    const requestID = response.headers.get('X-Request-ID'), retry = response.headers.get('Retry-After')
    throw new CartError(kind,status,write && kind === 'unavailable',requestID && /^req_[0-9a-f-]{36}$/.test(requestID) ? requestID : null,retry && /^(0|[1-9][0-9]{0,5})$/.test(retry) ? Number(retry) : null)
   }
   if (response.headers.get('content-type')?.split(';',1)[0]?.trim().toLowerCase() !== 'application/json' || response.headers.get('cache-control') !== 'private, no-store' || !response.body) {
    await response.body?.cancel().catch(()=>{})
    throw protocol()
   }
   const reader = response.body.getReader(), chunks: Uint8Array[] = []; let size = 0
   try {
    while (true) {
     const {done,value} = await reader.read()
     if (done) break
     size += value.byteLength
     if (size > maxBytes) throw protocol()
     chunks.push(value)
    }
   } finally { await reader.cancel().catch(()=>{}); reader.releaseLock() }
   const bytes = new Uint8Array(size); let offset = 0
   for (const chunk of chunks) { bytes.set(chunk,offset); offset += chunk.length }
   try { return {status,value:JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(bytes)) as unknown} }
   catch { throw protocol() }
  } catch (err) {
   if (err instanceof CartError) {
    if (write && err.kind === 'protocol') throw new CartError('protocol',err.status,true)
    throw err
   }
   throw new CartError('unavailable',null,write)
  } finally { clearTimeout(timer) }
 }

 // Call deliberately when beginning a new creation flow. It is NOT called
 // implicitly on every create/retry: scope loss must not create a new cart.
 async openScope(): Promise<CartScope> {
  const previous = this.readScopeCSRF()
  if (previous !== undefined && !matches(previous,tokenPattern)) throw new CartError('session')
  const {status,value} = await this.request('/api/store/cart-sessions','POST','{}',previous ? {'X-CSRF-Token':previous} : {})
  try {
   const v = record(value)
   if (status !== 200 || typeof v.reused !== 'boolean') throw protocol()
   keys(v,['csrf_token','reused'],v.reused ? [] : ['expires_at'])
   const token = text(v.csrf_token,tokenPattern)
   if (v.reused && token !== previous || !v.reused && !Object.hasOwn(v,'expires_at')) throw protocol()
   if (!v.reused) expiry(v.expires_at)
   return {csrfToken:token}
  } catch { throw new CartError('protocol',status,true) }
 }

 async create(input: CartCreation, scope: CartScope): Promise<CartCreated> {
  const {operationID,regionID,locale} = input, {csrfToken} = scope
  if (!matches(operationID,uuidPattern) || !matches(regionID,idPattern) || locale !== undefined && (typeof locale !== 'string' || locale !== '' && !matches(locale,localePattern) || locale.length > 35) || !matches(csrfToken,tokenPattern)) throw new CartError('input')
  // Explicit projection: runtime callers cannot send price/store/owner even if
  // they pass an object with extra fields. This does not mutate their input.
  const {status,value} = await this.request('/api/store/carts','POST',JSON.stringify({region_id:regionID,...(locale === undefined ? {} : {locale})}),{'X-CSRF-Token':csrfToken,'Idempotency-Key':operationID})
  try {
   const v = record(value)
   keys(v,['cart','csrf_token','expires_at','replayed','capabilities'])
   if (typeof v.replayed !== 'boolean' || status !== (v.replayed ? 200 : 201)) throw protocol()
   capabilities(v.capabilities)
   const cart = snapshot(v.cart)
   if (cart.region_id !== regionID || cart.locale !== (locale ?? '')) throw protocol()
   return {cart,csrfToken:text(v.csrf_token,tokenPattern),expiresAt:expiry(v.expires_at),replayed:v.replayed}
  } catch { throw new CartError('protocol',status,true) }
 }

 // The caller retains the operation and expected version on uncertainty.
 // A receipt is immutable historical evidence; retrieve current state before
 // a subsequent new operation. This method never retries or creates a cart.
 async addItem(id:string, input:CartItemAddition, authorization:{csrfToken:string}):Promise<CartItemReceipt> {
  const {operationID,version,variantID,quantity}=input, {csrfToken}=authorization
  if (!matches(id,idPattern) || !matches(operationID,uuidPattern) || !matches(version,versionPattern) || BigInt(version)>=18446744073709551615n || !matches(variantID,idPattern) || !matches(quantity,/^[1-9][0-9]{0,9}$/) || BigInt(quantity)>1000000000n || !matches(csrfToken,tokenPattern)) throw new CartError('input')
  const {status,value}=await this.request(`/api/store/carts/${encodeURIComponent(id)}/line-items`,'POST',JSON.stringify({version,variant_id:variantID,quantity}),{'X-CSRF-Token':csrfToken,'Idempotency-Key':operationID},65536,'item')
  try {
   const v=record(value)
   keys(v,['cart_id','version','line_id','quantity','replayed'])
   const next=text(v.version,versionPattern), merged=text(v.quantity,/^[1-9][0-9]{0,9}$/)
   if (status!==200 || v.cart_id!==id || BigInt(next)!==BigInt(version)+1n || BigInt(merged)<BigInt(quantity) || BigInt(merged)>1000000000n || typeof v.replayed!=='boolean') throw protocol()
   return {cart_id:id,version:next,line_id:text(v.line_id,idPattern),quantity:merged,replayed:v.replayed}
  } catch { throw new CartError('protocol',status,true) }
 }

 // Only explicit 401/404 means the ID is no longer accessible. Dependency,
 // network, timeout and malformed responses remain errors, never empty carts.
 async retrieve(id: string): Promise<CartSnapshot | null> {
  if (!matches(id,idPattern)) throw new CartError('input')
  let response: {status: number; value: unknown}
  try { response = await this.request(`/api/store/carts/${encodeURIComponent(id)}`,'GET') }
  catch (err) { if (err instanceof CartError && (err.status === 401 || err.status === 404)) return null; throw err }
  const v = record(response.value)
  keys(v,['cart','capabilities']); capabilities(v.capabilities)
  const cart = snapshot(v.cart)
  if (response.status !== 200 || cart.id !== id) throw protocol()
  return cart
 }

 // Persisted exact totals only. This never authorizes payment or claims that
 // current catalog prices/tax/shipping/promotion selection has been rerun.
 async retrieveAmounts(id: string): Promise<CartAmounts | null> {
  if (!matches(id,idPattern)) throw new CartError('input')
  let response: {status: number; value: unknown}
  try { response = await this.request(`/api/store/carts/${encodeURIComponent(id)}/amounts`,'GET',undefined,{},4*1024*1024) }
  catch (err) { if (err instanceof CartError && (err.status === 401 || err.status === 404)) return null; throw err }
  if (response.status !== 200) throw protocol()
  return amounts(response.value,id)
 }
}

export function browserCartClient(): CartClient {
 if (typeof window === 'undefined' || typeof document === 'undefined') throw new CartError('unavailable')
 return new CartClient({fetcher:window.fetch.bind(window),readScopeCSRF:() => scopeCSRFFromCookies(document.cookie,window.location.protocol === 'https:')})
}
