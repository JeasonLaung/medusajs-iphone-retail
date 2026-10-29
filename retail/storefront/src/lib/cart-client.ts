// Browser private transport for Gin. It deliberately shares neither the public
// server fetcher (which omits cookies) nor Medusa's SDK/authentication headers.
export type CartSnapshot = {
 id: string; region_id: string; sales_channel_id: string; currency_code: string
 locale: string; version: string
}
export type OrderConfirmation={
 id:string;display_id:string;custom_display_id:string;status:string;email:string;locale:string;created_at:string
 shipping_address:ContactAddress;billing_address:ContactAddress;items:CartViewItem[]
 amounts:{order_id:string;version:number;currency_code:string;totals:Record<string,string>;lines:{id:string;item_id:string;unit_price:string;quantity:string;totals:Record<string,string>}[];shipping_methods:{id:string;shipping_method_id:string;name:string;amount:string;is_tax_inclusive:boolean;totals:Record<string,string>}[];summary:{transaction_total:string;pending_difference:string}|null}
 payment:{provider_id:string;amount:string;currency_code:string;created_at:string}
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
export type CartViewItem = {id:string;title:string;product_id:string|null;product_title:string|null;product_handle:string|null;thumbnail:string|null;variant_title:string|null;option_values:Record<string,string>}
export type CartView = {amounts:CartAmounts;items:CartViewItem[]}
export type ContactAddress={first_name:string;last_name:string;company:string;address_1:string;address_2:string;city:string;country_code:string;province:string;postal_code:string;phone:string}
export type CartContactDetail={cart_id:string;version:string;email:string;shipping_address:ContactAddress|null;billing_address:ContactAddress|null}
export type CartContactInput={operationID:string;version:string;email:string;shipping_address:ContactAddress;billing_address:ContactAddress}
export type CartContactReceipt={cart_id:string;version:string;replayed:boolean}
export type DeliveryOption={id:string;profile_id:string;name:string;description:string;kind:'shipping'|'pickup';amount:string;is_tax_inclusive:boolean;insufficient_inventory:boolean}
export type DeliveryOptions={cart_id:string;version:string;currency_code:string;options:DeliveryOption[];selected_option_ids:string[];complete:boolean}
export type ShippingInput={operationID:string;version:string;optionID:string}
export type ShippingReceipt={cart_id:string;version:string;option_id:string;replayed:boolean}
export type CartCompletion={cart_id:string;order_id:string;version:string}
export type PaymentStatus={cart_id:string;version:string;state:'pending'|'authorized'|'paid_pending_order'|'failed'|'requires_review'}
const contactLimits={first_name:160,last_name:160,company:160,address_1:512,address_2:512,city:160,country_code:2,province:191,postal_code:64,phone:64} as const
export function contactAddress(value:unknown):ContactAddress {
 const v=record(value),out:Record<string,string>={}
 for(const [key,max] of Object.entries(contactLimits)){
  const text=v[key]
  if(typeof text!=='string'||[...text].length>max||/[\x00-\x1f\x7f]/.test(text))throw protocol()
  out[key]=text
 }
 if(!/^[a-z]{2}$/.test(out.country_code)||['first_name','last_name','address_1','city','postal_code'].some(k=>!out[k].trim()))throw protocol()
 return out as ContactAddress
}
function contactEmail(value:unknown):string {
 if(typeof value!=='string'||value.length>254||!value.includes('@')||value.trim()!==value||/[\x00-\x20\x7f<>]/.test(value))throw protocol()
 return value
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

 // Absolute quantity, including explicit zero removal. This shares the same
 // operation namespace and uncertainty contract as addition, never retries.
 async setItem(id:string,lineID:string,input:{operationID:string;version:string;quantity:string},authorization:{csrfToken:string}):Promise<CartItemReceipt>{
  const {operationID,version,quantity}=input,{csrfToken}=authorization
  if(!matches(id,idPattern)||!matches(lineID,idPattern)||!matches(operationID,uuidPattern)||!matches(version,versionPattern)||BigInt(version)>=18446744073709551615n||!matches(quantity,/^(0|[1-9][0-9]{0,9})$/)||BigInt(quantity)>1000000000n||!matches(csrfToken,tokenPattern))throw new CartError('input')
  const {status,value}=await this.request(`/api/store/carts/${encodeURIComponent(id)}/line-items/${encodeURIComponent(lineID)}`,'POST',JSON.stringify({version,quantity}),{'X-CSRF-Token':csrfToken,'Idempotency-Key':operationID},65536,'item')
  try{
   const v=record(value);keys(v,['cart_id','version','line_id','quantity','replayed'])
   if(status!==200||v.cart_id!==id||v.line_id!==lineID||v.quantity!==quantity||typeof v.replayed!=='boolean'||!matches(v.version,versionPattern)||BigInt(v.version)!==BigInt(version)+1n)throw protocol()
   return {cart_id:id,line_id:lineID,quantity,version:v.version,replayed:v.replayed}
  }catch{throw new CartError('protocol',status,true)}
 }

 // Empty intention: Gin resolves the saved paid attempt and authorizes even
 // closed-cart retries. No browser payment/order/status value is authoritative.
 async complete(id:string,authorization:{csrfToken:string}):Promise<CartCompletion>{
  const {csrfToken}=authorization
  if(!matches(id,idPattern)||!matches(csrfToken,tokenPattern))throw new CartError('input')
  const {status,value}=await this.request(`/api/store/carts/${encodeURIComponent(id)}/complete`,'POST','{}',{'X-CSRF-Token':csrfToken},65536,'item')
  try{
   const v=record(value);keys(v,['cart_id','order_id','version'])
   if(status!==200||v.cart_id!==id||!matches(v.order_id,idPattern)||!matches(v.version,versionPattern)||BigInt(v.version)>18446744073709551615n)throw protocol()
   return {cart_id:id,order_id:v.order_id,version:v.version}
  }catch{throw new CartError('protocol',status,true)}
 }

 async confirmation(id:string):Promise<OrderConfirmation|null>{
  if(!matches(id,idPattern))throw new CartError('input')
  let response:{status:number;value:unknown}
  try{response=await this.request(`/api/store/orders/${encodeURIComponent(id)}/confirmation`,'GET',undefined,{},4*1024*1024)}
  catch(error){if(error instanceof CartError&&(error.status===401||error.status===404))return null;throw error}
  const v=record(response.value);keys(v,['id','display_id','custom_display_id','status','email','locale','created_at','shipping_address','billing_address','items','amounts','payment'])
  const savedText=(value:unknown)=>{if(typeof value!=='string'||new TextEncoder().encode(value).length>4096||/[\x00-\x1f\x7f]/.test(value))throw protocol();return value}
  const date=(value:unknown)=>{const s=savedText(value);if(!/^\d{4}-\d\d-\d\dT/.test(s)||!Number.isFinite(Date.parse(s)))throw protocol();return s}
  const totalMap=(value:unknown)=>{const map=record(value);if(Object.keys(map).length>100||['total','original_total'].some(k=>typeof map[k]!=='string'))throw protocol();return Object.fromEntries(Object.entries(map).map(([k,n])=>{if(!/^[a-z_]{1,64}$/.test(k))throw protocol();return[k,text(n,amountPattern)]}))}
  const address=(value:unknown)=>{keys(record(value),Object.keys(contactLimits));return contactAddress(value)}
  if(response.status!==200||v.id!==id||!matches(v.display_id,/^[1-9][0-9]{0,18}$/)||!matches(v.custom_display_id,/^[0-9]{19}$/)||!['pending','completed','archived','canceled'].includes(String(v.status)))throw protocol()
  const a=record(v.amounts);if(['item_subtotal','shipping_subtotal','discount_subtotal','tax_total'].some(k=>typeof record(a.totals)[k]!=='string'))throw protocol();keys(a,['order_id','version','currency_code','totals','lines','shipping_methods','summary'])
  if(a.order_id!==id||!Number.isSafeInteger(a.version)||Number(a.version)<1||!matches(a.currency_code,/^[a-z]{3}$/)||!Array.isArray(a.lines)||!a.lines.length||a.lines.length>1000||!Array.isArray(a.shipping_methods)||a.shipping_methods.length>100)throw protocol()
  const itemIDs=new Set<string>(),lineIDs=new Set<string>()
  const lines=a.lines.map(value=>{const row=record(value);keys(row,['id','item_id','unit_price','quantity','totals']);const lineID=text(row.id,idPattern),itemID=text(row.item_id,idPattern);if(lineIDs.has(lineID)||itemIDs.has(itemID))throw protocol();lineIDs.add(lineID);itemIDs.add(itemID);return{id:lineID,item_id:itemID,unit_price:text(row.unit_price,amountPattern),quantity:text(row.quantity,amountPattern),totals:totalMap(row.totals)}})
  const shippingIDs=new Set<string>()
  const shipping_methods=a.shipping_methods.map(value=>{const row=record(value);keys(row,['id','shipping_method_id','name','amount','is_tax_inclusive','totals']);const method=text(row.shipping_method_id,idPattern);if(shippingIDs.has(method)||typeof row.is_tax_inclusive!=='boolean')throw protocol();shippingIDs.add(method);return{id:text(row.id,idPattern),shipping_method_id:method,name:savedText(row.name),amount:text(row.amount,amountPattern),is_tax_inclusive:row.is_tax_inclusive,totals:totalMap(row.totals)}})
  if(!Array.isArray(v.items)||v.items.length!==itemIDs.size)throw protocol()
  const nullable=(value:unknown)=>value===null?null:savedText(value)
  const items=v.items.map(value=>{const row=record(value);keys(row,['id','title','product_id','product_title','product_handle','thumbnail','variant_title','option_values']);const itemID=text(row.id,idPattern);if(!itemIDs.delete(itemID))throw protocol();const options=record(row.option_values);if(Object.keys(options).length>64)throw protocol();return{id:itemID,title:savedText(row.title),product_id:row.product_id===null?null:text(row.product_id,idPattern),product_title:nullable(row.product_title),product_handle:nullable(row.product_handle),thumbnail:nullable(row.thumbnail),variant_title:nullable(row.variant_title),option_values:Object.fromEntries(Object.entries(options).map(([k,v])=>[savedText(k),savedText(v)]))}})
  let summary:OrderConfirmation['amounts']['summary']=null
  if(a.summary!==null){const row=record(a.summary);keys(row,['transaction_total','pending_difference']);summary={transaction_total:text(row.transaction_total,amountPattern),pending_difference:text(row.pending_difference,amountPattern)}}
  const pay=record(v.payment);keys(pay,['provider_id','amount','currency_code','created_at']);if(pay.currency_code!==a.currency_code)throw protocol()
  return{id,display_id:v.display_id,custom_display_id:v.custom_display_id,status:String(v.status),email:contactEmail(v.email),locale:savedText(v.locale),created_at:date(v.created_at),shipping_address:address(v.shipping_address),billing_address:address(v.billing_address),items,amounts:{order_id:id,version:Number(a.version),currency_code:a.currency_code,totals:totalMap(a.totals),lines,shipping_methods,summary},payment:{provider_id:text(pay.provider_id,idPattern),amount:text(pay.amount,amountPattern),currency_code:a.currency_code,created_at:date(pay.created_at)}}
 }

 async paymentStatus(id:string,authorization:{csrfToken:string}):Promise<PaymentStatus>{
  const {csrfToken}=authorization
  if(!matches(id,idPattern)||!matches(csrfToken,tokenPattern))throw new CartError('input')
  const {status,value}=await this.request(`/api/store/carts/${encodeURIComponent(id)}/payment-status`,'POST','{}',{'X-CSRF-Token':csrfToken})
  const v=record(value);keys(v,['cart_id','version','state'])
  if(status!==200||v.cart_id!==id||!matches(v.version,versionPattern)||BigInt(v.version)>18446744073709551615n||!['pending','authorized','paid_pending_order','failed','requires_review'].includes(String(v.state)))throw protocol()
  return {cart_id:id,version:v.version,state:v.state as PaymentStatus['state']}
 }

 async saveShipping(id:string,input:ShippingInput,authorization:{csrfToken:string}):Promise<ShippingReceipt>{
  const {operationID,version,optionID}=input,{csrfToken}=authorization
  if(!matches(id,idPattern)||!matches(operationID,uuidPattern)||!matches(version,versionPattern)||BigInt(version)>=18446744073709551615n||!matches(optionID,idPattern)||!matches(csrfToken,tokenPattern))throw new CartError('input')
  const {status,value}=await this.request(`/api/store/carts/${encodeURIComponent(id)}/shipping-methods`,'POST',JSON.stringify({version,option_id:optionID}),{'X-CSRF-Token':csrfToken,'Idempotency-Key':operationID},65536,'item')
  try{
   const v=record(value);keys(v,['cart_id','version','option_id','replayed'])
   if(status!==200||v.cart_id!==id||v.option_id!==optionID||!matches(v.version,versionPattern)||BigInt(v.version)!==BigInt(version)+1n||typeof v.replayed!=='boolean')throw protocol()
   return {cart_id:id,version:v.version,option_id:optionID,replayed:v.replayed}
  }catch{throw new CartError('protocol',status,true)}
 }
 async retrieveDelivery(id:string):Promise<DeliveryOptions|null>{
  if(!matches(id,idPattern))throw new CartError('input')
  let response:{status:number;value:unknown}
  try{response=await this.request(`/api/store/carts/${encodeURIComponent(id)}/delivery-options`,'GET',undefined,{},1024*1024)}
  catch(err){if(err instanceof CartError&&(err.status===401||err.status===404))return null;throw err}
  const v=record(response.value);keys(v,['cart_id','version','currency_code','options','selected_option_ids','complete'])
  if(response.status!==200||v.cart_id!==id||!matches(v.version,versionPattern)||BigInt(v.version)>18446744073709551615n||!matches(v.currency_code,/^[a-z]{3}$/)||!Array.isArray(v.options)||v.options.length>100||!Array.isArray(v.selected_option_ids)||v.selected_option_ids.length>100||typeof v.complete!=='boolean')throw protocol()
  const savedText=(value:unknown):string=>{if(typeof value!=='string'||new TextEncoder().encode(value).length>4096||/[\x00-\x1f\x7f]/.test(value))throw protocol();return value}
  const ids=new Set<string>(),profiles=new Map<string,string>()
  const options=v.options.map(value=>{
   const o=record(value);keys(o,['id','profile_id','name','description','kind','amount','is_tax_inclusive','insufficient_inventory'])
   const optionID=text(o.id,idPattern),profileID=text(o.profile_id,idPattern)
   if(ids.has(optionID)||!['shipping','pickup'].includes(String(o.kind))||typeof o.is_tax_inclusive!=='boolean'||typeof o.insufficient_inventory!=='boolean'||!matches(o.amount,/^(0|[1-9][0-9]*)(\.[0-9]+)?$/)||o.amount.length>512)throw protocol()
   ids.add(optionID);profiles.set(optionID,profileID)
   return {id:optionID,profile_id:profileID,name:savedText(o.name),description:savedText(o.description),kind:o.kind as DeliveryOption['kind'],amount:o.amount,is_tax_inclusive:o.is_tax_inclusive,insufficient_inventory:o.insufficient_inventory}
  })
  const selected=new Set<string>(),selectedProfiles=new Set<string>()
  const selected_option_ids=v.selected_option_ids.map(value=>{const key=text(value,idPattern),profile=profiles.get(key);if(!profile||selected.has(key)||selectedProfiles.has(profile))throw protocol();selected.add(key);selectedProfiles.add(profile);return key})
  if(v.complete&&options.some(o=>!selectedProfiles.has(o.profile_id)))throw protocol()
  return {cart_id:id,version:v.version,currency_code:v.currency_code,options,selected_option_ids,complete:v.complete}
 }

 async saveContacts(id:string,input:CartContactInput,authorization:{csrfToken:string}):Promise<CartContactReceipt>{
  const {operationID,version}=input,{csrfToken}=authorization
  let payload:unknown
  try{
   if(!matches(id,idPattern)||!matches(operationID,uuidPattern)||!matches(version,versionPattern)||BigInt(version)>=18446744073709551615n||!matches(csrfToken,tokenPattern))throw protocol()
   payload={version,email:contactEmail(input.email),shipping_address:contactAddress(input.shipping_address),billing_address:contactAddress(input.billing_address)}
  }catch{throw new CartError('input')}
  const {status,value}=await this.request(`/api/store/carts/${encodeURIComponent(id)}/contacts`,'POST',JSON.stringify(payload),{'X-CSRF-Token':csrfToken,'Idempotency-Key':operationID},65536,'item')
  try{
   const v=record(value);keys(v,['cart_id','version','replayed'])
   if(status!==200||v.cart_id!==id||!matches(v.version,versionPattern)||BigInt(v.version)!==BigInt(version)+1n||typeof v.replayed!=='boolean')throw protocol()
   return {cart_id:id,version:v.version,replayed:v.replayed}
  }catch{throw new CartError('protocol',status,true)}
 }
 async retrieveContacts(id:string):Promise<CartContactDetail|null>{
  if(!matches(id,idPattern))throw new CartError('input')
  let response:{status:number;value:unknown}
  try{response=await this.request(`/api/store/carts/${encodeURIComponent(id)}/contacts`,'GET')}
  catch(err){if(err instanceof CartError&&(err.status===401||err.status===404))return null;throw err}
  const v=record(response.value);keys(v,['cart_id','version','email','shipping_address','billing_address'])
  if(response.status!==200||v.cart_id!==id||!matches(v.version,versionPattern)||BigInt(v.version)>18446744073709551615n)throw protocol()
  return {cart_id:id,version:v.version,email:v.email===''?'':contactEmail(v.email),shipping_address:v.shipping_address===null?null:contactAddress(v.shipping_address),billing_address:v.billing_address===null?null:contactAddress(v.billing_address)}
 }

 // Only explicit 401/404 means the ID is no longer accessible. Dependency,
 // network, timeout and malformed responses remain errors, never empty carts.
 async retrieve(id: string): Promise<CartSnapshot | null> {
  if (!matches(id,idPattern)) throw new CartError('input')
  return this.readSnapshot(`/api/store/carts/${encodeURIComponent(id)}`,id)
 }

 // Restore from HttpOnly possession without a saved/URL-selected cart ID.
 // No creation, scope issuance or TTL refresh occurs on this read.
 async current():Promise<CartSnapshot|null> {
  return this.readSnapshot('/api/store/cart')
 }

 private async readSnapshot(path:string, expectedID?:string):Promise<CartSnapshot|null> {
  let response: {status: number; value: unknown}
  try { response = await this.request(path,'GET') }
  catch (err) { if (err instanceof CartError && (err.status === 401 || err.status === 404)) return null; throw err }
  const v = record(response.value)
  keys(v,['cart','capabilities']); capabilities(v.capabilities)
  const cart = snapshot(v.cart)
  if (response.status !== 200 || expectedID !== undefined && cart.id !== expectedID) throw protocol()
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

 async retrieveView(id:string):Promise<CartView|null> {
  if(!matches(id,idPattern))throw new CartError('input')
  let response:{status:number;value:unknown}
  try{response=await this.request(`/api/store/carts/${encodeURIComponent(id)}/view`,'GET',undefined,{},4*1024*1024)}
  catch(err){if(err instanceof CartError&&(err.status===401||err.status===404))return null;throw err}
  const root=record(response.value);keys(root,['amounts','items'])
  const detail=amounts(root.amounts,id)
  if(response.status!==200||!Array.isArray(root.items)||root.items.length!==detail.lines.length)throw protocol()
  const wanted=new Set(detail.lines.map(line=>line.id))
  const savedText=(v:unknown):string=>{
   if(typeof v!=='string'||new TextEncoder().encode(v).length>4096||/[\x00-\x1f\x7f]/.test(v))throw protocol()
   return v
  }
  const nullable=(v:unknown)=>v===null?null:savedText(v)
  const items=root.items.map(value=>{
   const row=record(value);keys(row,['id','title','product_id','product_title','product_handle','thumbnail','variant_title','option_values'])
   const lineID=text(row.id,idPattern)
   if(!wanted.delete(lineID))throw protocol()
   const options=record(row.option_values)
   if(Object.keys(options).length>64)throw protocol()
   const option_values=Object.fromEntries(Object.entries(options).map(([k,v])=>[savedText(k),savedText(v)]))
   return {id:lineID,title:savedText(row.title),product_id:row.product_id===null?null:text(row.product_id,idPattern),product_title:nullable(row.product_title),product_handle:nullable(row.product_handle),thumbnail:nullable(row.thumbnail),variant_title:nullable(row.variant_title),option_values}
  })
  return {amounts:detail,items}
 }
}

export function browserCartClient(): CartClient {
 if (typeof window === 'undefined' || typeof document === 'undefined') throw new CartError('unavailable')
 return new CartClient({fetcher:window.fetch.bind(window),readScopeCSRF:() => scopeCSRFFromCookies(document.cookie,window.location.protocol === 'https:')})
}
