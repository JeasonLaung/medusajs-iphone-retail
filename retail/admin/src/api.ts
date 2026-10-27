export type User = { id: string; email: string; name: string; role: string; enabled: boolean }

export class APIError extends Error {
  constructor(readonly status: number, readonly code: string, readonly requestID = '') {
    super(code)
  }
}

function csrfCookie() {
  // Secure host cookie takes precedence; never put a session token in JS storage.
  for (const name of ['__Host-retail_admin_csrf', 'retail_admin_csrf']) {
    const matches = document.cookie.split(';').map(c => c.trim()).filter(c => c.startsWith(`${name}=`))
    if (matches.length === 1) {
      const value = matches[0].slice(name.length + 1)
      if (/^[a-f0-9]{64}$/.test(value)) return value
    }
  }
  return ''
}

async function request<T>(path: string, options: { method?: 'GET' | 'POST' | 'PATCH'; body?: unknown; csrf?: string; signal?: AbortSignal } = {}, base = '/api/admin/auth/'): Promise<T> {
  const controller = new AbortController()
  const abort = () => controller.abort()
  options.signal?.addEventListener('abort', abort, { once: true })
  if (options.signal?.aborted) controller.abort()
  const timer = window.setTimeout(abort, 10_000)
  try {
    const response = await fetch(`${base}${path}`, {
      method: options.method ?? 'GET', credentials: 'same-origin', cache: 'no-store',
      redirect: 'error', signal: controller.signal,
      headers: { 'Content-Type': 'application/json', ...(options.csrf ? { 'X-CSRF-Token': options.csrf } : {}) },
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    })
    const data: unknown = await response.json().catch(() => null)
    if (!response.ok) {
      const error = data && typeof data === 'object' && 'error' in data ? data.error : null
      const shape = error && typeof error === 'object' ? error as Record<string, unknown> : {}
      throw new APIError(response.status, typeof shape.code === 'string' ? shape.code : 'request.failed',
        typeof shape.request_id === 'string' && /^req_[a-f0-9-]{36}$/.test(shape.request_id) ? shape.request_id : '')
    }
    return data as T
  } catch (error) {
    if (error instanceof APIError) throw error
    throw new APIError(0, 'request.unavailable')
  } finally {
    window.clearTimeout(timer)
    options.signal?.removeEventListener('abort', abort)
  }
}

export type ProductSummary = { id: string; title: string; handle: string; status: 'draft' | 'proposed' | 'published' | 'rejected'; thumbnail: string | null; collection_id: string | null; created_at: string; updated_at: string; variant_count: number }
export type ProductList = { products: ProductSummary[]; count: number; limit: number; offset: number }
export type ProductImage = {id:string;url:string;rank:number}
export type ProductOption = {id:string;title:string;values:{id:string;value:string;rank:number|null}[]}
export type ProductDetail = ProductSummary & {revision:string;subtitle:string|null;description:string|null;options:ProductOption[];images:ProductImage[]}
export type ProductEdit = {operation_id:string;expected_revision:string;title:string;subtitle:string|null;description:string|null;status:ProductSummary['status']}
export async function editProduct(id:string,body:ProductEdit):Promise<void> {
  const csrf=csrfCookie()
  if(!csrf)throw new APIError(403,'auth.invalid_csrf')
  const result=await request<{revision:string;replayed:boolean}>(`products/${encodeURIComponent(id)}`,{method:'PATCH',csrf,body},'/api/admin/')
  if(!result||typeof result.revision!=='string'||!/^[a-f0-9]{64}$/.test(result.revision)||typeof result.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
}
export type VariantDetail = {id:string;product_id:string;title:string;sku:string|null;thumbnail:string|null;variant_rank:number|null;manage_inventory:boolean;allow_backorder:boolean;options:{id:string;option_id:string;value:string}[];unassigned_option_value_ids:string[];configuration_complete:boolean;images:ProductImage[]}
export type VariantList = {variants:VariantDetail[];count:number;limit:number;offset:number}
export type InventoryLevel={revision:string;id:string;location_id:string;location_name:string|null;stocked_quantity:string;reserved_quantity:string;incoming_quantity:string;available_quantity:string;reservation_quantity:string;reservation_count:number;reconciled:boolean}
export type InventoryHold={location_id:string;reason:'reservation_mismatch'|'negative_balance'|'unmatched_reservations'}
export type InventoryItem={migration_holds:InventoryHold[];inventory_item_id:string;required_quantity:string;sku:string|null;title:string|null;item_available:boolean;levels:InventoryLevel[];unmatched_reservations:{location_id:string;quantity:string;count:number}[]}
export type VariantInventory={product_id:string;variant_id:string;manage_inventory:boolean;allow_backorder:boolean;writes_enabled:false;stock_edit_enabled:boolean;items:InventoryItem[]}
const inventoryDecimal=(v:unknown):v is string=>typeof v==='string'&&/^-?(?:0|[1-9]\d{0,46})(?:\.\d{1,18})?$/.test(v)
const safeCount=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0
export async function variantInventory(productID:string,variantID:string,signal:AbortSignal):Promise<VariantInventory>{
  const d=await request<VariantInventory>(`products/${encodeURIComponent(productID)}/variants/${encodeURIComponent(variantID)}/inventory`,{signal},'/api/admin/')
  if(!d||d.product_id!==productID||d.variant_id!==variantID||typeof d.manage_inventory!=='boolean'||typeof d.allow_backorder!=='boolean'||d.writes_enabled!==false||typeof d.stock_edit_enabled!=='boolean'||!Array.isArray(d.items)||d.items.length>1000)throw new APIError(0,'request.invalid_response')
  const items=d.items.map(i=>{
    if(!i||typeof i.inventory_item_id!=='string'||!inventoryDecimal(i.required_quantity)||!nullableText(i.sku)||!nullableText(i.title)||typeof i.item_available!=='boolean'||!Array.isArray(i.migration_holds)||i.migration_holds.length>30000||!i.migration_holds.every(h=>h&&typeof h.location_id==='string'&&['reservation_mismatch','negative_balance','unmatched_reservations'].includes(h.reason))||!Array.isArray(i.levels)||i.levels.length>10000||!Array.isArray(i.unmatched_reservations)||i.unmatched_reservations.length>10000||!i.levels.every(l=>l&&typeof l.revision==='string'&&/^[a-f0-9]{64}$/.test(l.revision)&&typeof l.id==='string'&&typeof l.location_id==='string'&&nullableText(l.location_name)&&[l.stocked_quantity,l.reserved_quantity,l.incoming_quantity,l.available_quantity,l.reservation_quantity].every(inventoryDecimal)&&safeCount(l.reservation_count)&&typeof l.reconciled==='boolean')||!i.unmatched_reservations.every(r=>r&&typeof r.location_id==='string'&&inventoryDecimal(r.quantity)&&safeCount(r.count)))throw new APIError(0,'request.invalid_response')
    return {migration_holds:i.migration_holds.map(({location_id,reason})=>({location_id,reason})),inventory_item_id:i.inventory_item_id,required_quantity:i.required_quantity,sku:i.sku,title:i.title,item_available:i.item_available,levels:i.levels.map(({revision,id,location_id,location_name,stocked_quantity,reserved_quantity,incoming_quantity,available_quantity,reservation_quantity,reservation_count,reconciled})=>({revision,id,location_id,location_name,stocked_quantity,reserved_quantity,incoming_quantity,available_quantity,reservation_quantity,reservation_count,reconciled})),unmatched_reservations:i.unmatched_reservations.map(({location_id,quantity,count})=>({location_id,quantity,count}))}
  })
  return {product_id:d.product_id,variant_id:d.variant_id,manage_inventory:d.manage_inventory,allow_backorder:d.allow_backorder,writes_enabled:false,stock_edit_enabled:d.stock_edit_enabled,items}
}
export type PriceRule={id:string;attribute:string;operator:string;value:string;priority:number}
export type PriceListRule={id:string;attribute:string;value:string|null}
export type PriceList={id:string;title:string;status:'active'|'draft';type:'sale'|'override';starts_at:string|null;ends_at:string|null;rules_count:number|null;rules:PriceListRule[]}
export type ConfiguredPrice={id:string;price_set_id:string;currency_code:string;amount:string;min_quantity:string|null;max_quantity:string|null;rules_count:number|null;price_list_id:string|null;price_list:PriceList|null;rules:PriceRule[]}
export type VariantPrices={product_id:string;variant_id:string;price_set_ids:string[];prices:ConfiguredPrice[];count:number;limit:number;offset:number}
const decimalText=(value:unknown):value is string=>typeof value==='string'&&/^-?(?:0|[1-9]\d{0,19})(?:\.\d{1,18})?$/.test(value)
const nullableDecimal=(value:unknown)=>value===null||decimalText(value)
const nullableCount=(value:unknown)=>value===null||(typeof value==='number'&&Number.isSafeInteger(value)&&value>=0)
const nullableTime=(value:unknown)=>value===null||(typeof value==='string'&&Number.isFinite(Date.parse(value)))
export async function variantPrices(productID:string,variantID:string,limit:number,offset:number,signal:AbortSignal):Promise<VariantPrices>{
  const d=await request<VariantPrices>(`products/${encodeURIComponent(productID)}/variants/${encodeURIComponent(variantID)}/prices?limit=${limit}&offset=${offset}`,{signal},'/api/admin/')
  if(!d||d.product_id!==productID||d.variant_id!==variantID||d.limit!==limit||d.offset!==offset||!Number.isSafeInteger(d.count)||d.count<0||!Array.isArray(d.price_set_ids)||d.price_set_ids.length>1000||!d.price_set_ids.every(s=>typeof s==='string')||new Set(d.price_set_ids).size!==d.price_set_ids.length||!Array.isArray(d.prices)||d.prices.length>limit)throw new APIError(0,'request.invalid_response')
  const sets=new Set(d.price_set_ids)
  const prices=d.prices.map(p=>{
    if(!p||typeof p.id!=='string'||!sets.has(p.price_set_id)||typeof p.currency_code!=='string'||!decimalText(p.amount)||!nullableDecimal(p.min_quantity)||!nullableDecimal(p.max_quantity)||!nullableCount(p.rules_count)||!nullableText(p.price_list_id)||!Array.isArray(p.rules)||p.rules.length>10000||!p.rules.every(r=>r&&typeof r.id==='string'&&typeof r.attribute==='string'&&['gte','lte','gt','lt','eq'].includes(r.operator)&&typeof r.value==='string'&&Number.isSafeInteger(r.priority)))throw new APIError(0,'request.invalid_response')
    const l=p.price_list
    if(l!==null&&(!l||l.id!==p.price_list_id||typeof l.title!=='string'||!['active','draft'].includes(l.status)||!['sale','override'].includes(l.type)||!nullableTime(l.starts_at)||!nullableTime(l.ends_at)||!nullableCount(l.rules_count)||!Array.isArray(l.rules)||l.rules.length>10000||!l.rules.every(r=>r&&typeof r.id==='string'&&typeof r.attribute==='string'&&nullableText(r.value))))throw new APIError(0,'request.invalid_response')
    return {id:p.id,price_set_id:p.price_set_id,currency_code:p.currency_code,amount:p.amount,min_quantity:p.min_quantity,max_quantity:p.max_quantity,rules_count:p.rules_count,price_list_id:p.price_list_id,rules:p.rules.map(({id,attribute,operator,value,priority})=>({id,attribute,operator,value,priority})),price_list:l===null?null:{id:l.id,title:l.title,status:l.status,type:l.type,starts_at:l.starts_at,ends_at:l.ends_at,rules_count:l.rules_count,rules:l.rules.map(({id,attribute,value})=>({id,attribute,value}))}}
  })
  return {product_id:d.product_id,variant_id:d.variant_id,price_set_ids:[...d.price_set_ids],prices,count:d.count,limit:d.limit,offset:d.offset}
}
const nullableText=(value:unknown)=>value===null||typeof value==='string'
const imagesValid=(images:ProductImage[])=>Array.isArray(images)&&images.every(i=>i&&typeof i.id==='string'&&typeof i.url==='string'&&Number.isSafeInteger(i.rank))
const projectImages=(images:ProductImage[])=>images.map(({id,url,rank})=>({id,url,rank}))
export async function productDetail(id:string,signal:AbortSignal):Promise<ProductDetail> {
  const data=await request<{product:ProductDetail}>(`products/${encodeURIComponent(id)}`,{signal},'/api/admin/')
  const p=data?.product
  if(typeof p?.revision!=='string'||!/^[a-f0-9]{64}$/.test(p.revision))throw new APIError(0,'request.invalid_response')
  if(!p||p.id!==id||typeof p.title!=='string'||typeof p.handle!=='string'||!['draft','proposed','published','rejected'].includes(p.status)||!nullableText(p.subtitle)||!nullableText(p.description)||!nullableText(p.thumbnail)||!nullableText(p.collection_id)||typeof p.created_at!=='string'||typeof p.updated_at!=='string'||!Number.isFinite(Date.parse(p.created_at))||!Number.isFinite(Date.parse(p.updated_at))||!Number.isSafeInteger(p.variant_count)||p.variant_count<0||!Array.isArray(p.options)||!imagesValid(p.images)||!p.options.every(o=>o&&typeof o.id==='string'&&typeof o.title==='string'&&Array.isArray(o.values)&&o.values.every(v=>v&&typeof v.id==='string'&&typeof v.value==='string'&&(v.rank===null||Number.isSafeInteger(v.rank)))))throw new APIError(0,'request.invalid_response')
  return {id:p.id,title:p.title,handle:p.handle,status:p.status,thumbnail:p.thumbnail,collection_id:p.collection_id,created_at:p.created_at,updated_at:p.updated_at,variant_count:p.variant_count,revision:p.revision,subtitle:p.subtitle,description:p.description,images:projectImages(p.images),options:p.options.map(({id,title,values})=>({id,title,values:values.map(({id,value,rank})=>({id,value,rank}))}))}
}
export async function productVariants(id:string,query:URLSearchParams,signal:AbortSignal):Promise<VariantList>{
  const data=await request<VariantList>(`products/${encodeURIComponent(id)}/variants?${query}`,{signal},'/api/admin/')
  if(!data||!Number.isSafeInteger(data.count)||data.count<0||!Number.isSafeInteger(data.limit)||data.limit<1||data.limit>100||!Number.isSafeInteger(data.offset)||data.offset<0||!Array.isArray(data.variants)||data.variants.length>data.limit||!data.variants.every(v=>v&&typeof v.id==='string'&&v.product_id===id&&typeof v.title==='string'&&nullableText(v.sku)&&nullableText(v.thumbnail)&&(v.variant_rank===null||Number.isSafeInteger(v.variant_rank))&&typeof v.manage_inventory==='boolean'&&typeof v.allow_backorder==='boolean'&&typeof v.configuration_complete==='boolean'&&Array.isArray(v.unassigned_option_value_ids)&&v.unassigned_option_value_ids.every(x=>typeof x==='string')&&imagesValid(v.images)&&Array.isArray(v.options)&&v.options.every(o=>o&&typeof o.id==='string'&&typeof o.option_id==='string'&&typeof o.value==='string')))throw new APIError(0,'request.invalid_response')
  return {count:data.count,limit:data.limit,offset:data.offset,variants:data.variants.map(v=>({id:v.id,product_id:v.product_id,title:v.title,sku:v.sku,thumbnail:v.thumbnail,variant_rank:v.variant_rank,manage_inventory:v.manage_inventory,allow_backorder:v.allow_backorder,configuration_complete:v.configuration_complete,unassigned_option_value_ids:[...v.unassigned_option_value_ids],options:v.options.map(({id,option_id,value})=>({id,option_id,value})),images:projectImages(v.images)}))}
}
export async function listProducts(query: URLSearchParams, signal: AbortSignal): Promise<ProductList> {
  const data = await request<ProductList>(`products?${query}`, { signal }, '/api/admin/')
  if (!data || !Number.isSafeInteger(data.count) || data.count < 0 || !Number.isSafeInteger(data.limit) || data.limit < 1 || data.limit > 100 || !Number.isSafeInteger(data.offset) || data.offset < 0 || !Array.isArray(data.products) || data.products.length > data.limit) throw new APIError(0, 'request.invalid_response')
  const products = data.products.map(p => {
    if (!p || typeof p.id !== 'string' || !p.id || typeof p.title !== 'string' || typeof p.handle !== 'string' || !['draft','proposed','published','rejected'].includes(p.status) || !Number.isSafeInteger(p.variant_count) || p.variant_count < 0 || typeof p.created_at !== 'string' || typeof p.updated_at !== 'string' || !Number.isFinite(Date.parse(p.created_at)) || !Number.isFinite(Date.parse(p.updated_at)) || (p.thumbnail !== null && typeof p.thumbnail !== 'string') || (p.collection_id !== null && typeof p.collection_id !== 'string')) throw new APIError(0, 'request.invalid_response')
    return { id:p.id,title:p.title,handle:p.handle,status:p.status,thumbnail:p.thumbnail,collection_id:p.collection_id,created_at:p.created_at,updated_at:p.updated_at,variant_count:p.variant_count }
  })
  return { products, count:data.count,limit:data.limit,offset:data.offset }
}

function account(data: unknown): User {
  if (!data || typeof data !== 'object' || !('user' in data)) throw new APIError(0, 'request.invalid_response')
  const u = data.user as Partial<User> | null
  if (!u || typeof u.id !== 'string' || !u.id || typeof u.email !== 'string' || typeof u.name !== 'string' || u.role !== 'admin' || u.enabled !== true) {
    throw new APIError(0, 'request.invalid_response')
  }
  return { id: u.id, email: u.email, name: u.name, role: u.role, enabled: u.enabled }
}

export const auth = {
  async me(signal?: AbortSignal) { return account(await request('me', { signal })) },
  async login(email: string, password: string) {
    // Obtain a fresh one-use challenge for every attempt. Never retry a write.
    const challenge = await request<{ csrf_token: string }>('challenge', { method: 'POST' })
    if (!challenge || !/^[a-f0-9]{64}$/.test(challenge.csrf_token)) throw new APIError(0, 'request.invalid_response')
    return account(await request('login', { method: 'POST', csrf: challenge.csrf_token, body: { email, password } }))
  },
  async logout() {
    const csrf = csrfCookie()
    if (!csrf) throw new APIError(403, 'auth.invalid_csrf')
    const result = await request<{ success: boolean }>('logout', { method: 'POST', csrf })
    if (result?.success !== true) throw new APIError(0, 'request.invalid_response')
  },
}

export type StockEdit={operation_id:string;expected_revision:string;stocked_quantity:string;incoming_quantity:string}
export function stockQuantity(value:unknown):string|undefined {
  if(typeof value!=='string'||!/^(?:0|[1-9]\d{0,19})(?:\.\d{1,18})?$/.test(value))return
  const [whole,fraction='']=value.split('.')
  return whole+'.'+fraction.padEnd(18,'0')
}
export async function editStock(levelID:string,body:StockEdit):Promise<{revision:string;replayed:boolean}>{
  const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
  const result=await request<{revision:string;replayed:boolean}>(`inventory-levels/${encodeURIComponent(levelID)}/stock`,{method:'POST',body,csrf},'/api/admin/')
  if(!result||typeof result.revision!=='string'||!/^[a-f0-9]{64}$/.test(result.revision)||typeof result.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
  return {revision:result.revision,replayed:result.replayed}
}

export type OrderSummary = {id:string;display_id:string|null;custom_display_id:string|null;version:number;status:string;is_draft_order:boolean;email:string|null;currency_code:string;created_at:string;updated_at:string;line_count:number}
export type OrderList = {orders:OrderSummary[];count:number;limit:number;offset:number}
export type OrderLine = {id:string;item_id:string;title:string;variant_sku:string|null;quantity:string;unit_price:string;is_tax_inclusive:boolean;fulfilled_quantity:string;shipped_quantity:string;return_requested_quantity:string;return_received_quantity:string;return_dismissed_quantity:string}
export type OrderDetail = {order:OrderSummary;lines:OrderLine[]}
export function listOrders(params:URLSearchParams,signal?:AbortSignal):Promise<OrderList> {return request(`orders?${params}`,{signal},'/api/admin/')}
export function getOrder(id:string,signal?:AbortSignal):Promise<OrderDetail> {return request(`orders/${encodeURIComponent(id)}`,{signal},'/api/admin/')}
export type OrderAmounts = {order_id:string;version:number;currency_code:string;totals:Record<string,string>;lines:{id:string;item_id:string;unit_price:string;quantity:string;totals:Record<string,string>}[];shipping_methods:{id:string;shipping_method_id:string;name:string;amount:string;is_tax_inclusive:boolean;totals:Record<string,string>}[];summary:{transaction_total:string;pending_difference:string}|null}
export function getOrderAmounts(id:string,signal?:AbortSignal):Promise<OrderAmounts> {return request(`orders/${encodeURIComponent(id)}/amounts`,{signal},'/api/admin/')}

export type PaymentMovement={id:string;amount:string;created_at:string}
export type PaymentRecord={id:string;provider_id:string;session_id:string;currency_code:string;amount:string;captured_at:string|null;canceled_at:string|null;operation_hold:'held'|'unverified';captures:PaymentMovement[];refunds:PaymentMovement[]}
export type PaymentSession={id:string;provider_id:string;currency_code:string;amount:string;status:string;operation_hold:'held'|'unverified'}
export type PaymentCollection={id:string;currency_code:string;status:string;amount:string;authorized_amount:string|null;captured_amount:string|null;refunded_amount:string|null;payments:PaymentRecord[];sessions:PaymentSession[]}
export type OrderPayments={order_id:string;version:number;operations_enabled:false;collections:PaymentCollection[]}
export async function getOrderPayments(id:string,signal?:AbortSignal):Promise<OrderPayments>{
 const d=await request<OrderPayments>(`orders/${encodeURIComponent(id)}/payments`,{signal},'/api/admin/')
 const text=(v:unknown)=>typeof v==='string'&&v.length>0
 const money=(v:unknown)=>typeof v==='string'&&/^-?(?:0|[1-9]\d{0,127})(?:\.\d{1,64})?$/.test(v)
 const held=(v:unknown)=>v==='held'||v==='unverified'
 const movements=(v:PaymentMovement[])=>Array.isArray(v)&&v.length<=1000&&v.every(m=>m&&text(m.id)&&money(m.amount)&&typeof m.created_at==='string'&&Number.isFinite(Date.parse(m.created_at)))
 if(!d||d.order_id!==id||!Number.isSafeInteger(d.version)||d.version<1||d.operations_enabled!==false||!Array.isArray(d.collections)||d.collections.length>100||!d.collections.every(c=>c&&text(c.id)&&text(c.currency_code)&&text(c.status)&&money(c.amount)&&[c.authorized_amount,c.captured_amount,c.refunded_amount].every(v=>v===null||money(v))&&Array.isArray(c.sessions)&&c.sessions.length<=1000&&c.sessions.every(v=>v&&text(v.id)&&text(v.provider_id)&&text(v.currency_code)&&text(v.status)&&money(v.amount)&&held(v.operation_hold))&&Array.isArray(c.payments)&&c.payments.length<=1000&&c.payments.every(v=>v&&text(v.id)&&text(v.provider_id)&&text(v.session_id)&&text(v.currency_code)&&money(v.amount)&&nullableTime(v.captured_at)&&nullableTime(v.canceled_at)&&held(v.operation_hold)&&movements(v.captures)&&movements(v.refunds))))throw new APIError(0,'request.invalid_response')
 // Explicit projection prevents unexpected private extensions entering UI state.
 return {order_id:d.order_id,version:d.version,operations_enabled:false,collections:d.collections.map(c=>({id:c.id,currency_code:c.currency_code,status:c.status,amount:c.amount,authorized_amount:c.authorized_amount,captured_amount:c.captured_amount,refunded_amount:c.refunded_amount,sessions:c.sessions.map(({id,provider_id,currency_code,amount,status,operation_hold})=>({id,provider_id,currency_code,amount,status,operation_hold})),payments:c.payments.map(p=>({id:p.id,provider_id:p.provider_id,session_id:p.session_id,currency_code:p.currency_code,amount:p.amount,captured_at:p.captured_at,canceled_at:p.canceled_at,operation_hold:p.operation_hold,captures:p.captures.map(({id,amount,created_at})=>({id,amount,created_at})),refunds:p.refunds.map(({id,amount,created_at})=>({id,amount,created_at}))}))}))}
}


export type FulfillmentAddress={id:string;company:string|null;first_name:string|null;last_name:string|null;address_1:string|null;address_2:string|null;city:string|null;country_code:string|null;province:string|null;postal_code:string|null;phone:string|null}
export type FulfillmentItem={id:string;title:string;sku:string;barcode:string;quantity:string;line_item_id:string|null;inventory_item_id:string|null}
export type FulfillmentLabel={id:string;tracking_number:string;tracking_url:string;label_url:string}
export type FulfillmentRecord={location_id:string;provider_id:string|null;shipping_option_id:string|null;delivery_address_id:string|null;requires_shipping:boolean;created_at:string;packed_at:string|null;shipped_at:string|null;delivered_at:string|null;canceled_at:string|null;address:FulfillmentAddress|null;items:FulfillmentItem[];labels:FulfillmentLabel[]}
export type FulfillmentLink={id:string;availability:'available'|'missing'|'deleted';record:FulfillmentRecord|null}
export type OrderFulfillments={order_id:string;version:number;operations_enabled:false;fulfillments:FulfillmentLink[]}
export async function getOrderFulfillments(id:string,signal?:AbortSignal):Promise<OrderFulfillments>{
 const d=await request<OrderFulfillments>(`orders/${encodeURIComponent(id)}/fulfillments`,{signal},'/api/admin/')
 const text=(v:unknown):v is string=>typeof v==='string'
 const optional=(v:unknown)=>v===null||text(v)
 const quantity=(v:unknown)=>text(v)&&/^-?(?:0|[1-9]\d{0,127})(?:\.\d{1,64})?$/.test(v)
 const address=(a:FulfillmentAddress|null)=>a===null||!!(a&&text(a.id)&&a.id.length>0&&[a.company,a.first_name,a.last_name,a.address_1,a.address_2,a.city,a.country_code,a.province,a.postal_code,a.phone].every(optional))
 let itemCount=0,labelCount=0
 if(!d||d.order_id!==id||!Number.isSafeInteger(d.version)||d.version<1||d.operations_enabled!==false||!Array.isArray(d.fulfillments)||d.fulfillments.length>100||!d.fulfillments.every(f=>{
  if(!f||!text(f.id)||f.id.length===0)return false
  if(f.availability==='missing'||f.availability==='deleted')return f.record===null
  const r=f.record
  if(f.availability!=='available'||!r||!text(r.location_id)||r.location_id.length===0||![r.provider_id,r.shipping_option_id,r.delivery_address_id].every(optional)||typeof r.requires_shipping!=='boolean'||!text(r.created_at)||!Number.isFinite(Date.parse(r.created_at))||![r.packed_at,r.shipped_at,r.delivered_at,r.canceled_at].every(nullableTime)||!address(r.address)||r.address!==null&&r.address.id!==r.delivery_address_id||!Array.isArray(r.items)||!Array.isArray(r.labels))return false
  itemCount+=r.items.length;labelCount+=r.labels.length
  return itemCount<=1000&&labelCount<=1000&&r.items.every(i=>i&&text(i.id)&&i.id.length>0&&[i.title,i.sku,i.barcode].every(text)&&quantity(i.quantity)&&[i.line_item_id,i.inventory_item_id].every(optional))&&r.labels.every(l=>l&&text(l.id)&&l.id.length>0&&[l.tracking_number,l.tracking_url,l.label_url].every(text))
 }))throw new APIError(0,'request.invalid_response')
 const seen=new Set<string>();for(const f of d.fulfillments){if(seen.has(f.id))throw new APIError(0,'request.invalid_response');seen.add(f.id)}
 return {order_id:d.order_id,version:d.version,operations_enabled:false,fulfillments:d.fulfillments.map(f=>{
  const r=f.record,a=r?.address
  return {id:f.id,availability:f.availability,record:r?{location_id:r.location_id,provider_id:r.provider_id,shipping_option_id:r.shipping_option_id,delivery_address_id:r.delivery_address_id,requires_shipping:r.requires_shipping,created_at:r.created_at,packed_at:r.packed_at,shipped_at:r.shipped_at,delivered_at:r.delivered_at,canceled_at:r.canceled_at,address:a?{id:a.id,company:a.company,first_name:a.first_name,last_name:a.last_name,address_1:a.address_1,address_2:a.address_2,city:a.city,country_code:a.country_code,province:a.province,postal_code:a.postal_code,phone:a.phone}:null,items:r.items.map(({id,title,sku,barcode,quantity,line_item_id,inventory_item_id})=>({id,title,sku,barcode,quantity,line_item_id,inventory_item_id})),labels:r.labels.map(({id,tracking_number,tracking_url,label_url})=>({id,tracking_number,tracking_url,label_url}))}:null}
 })}
}


export const ACTIONS=['authorization.read','authorization.write','inventory.read','inventory.stock.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','product.read','product.write'] as const
export type Action=typeof ACTIONS[number]
export type ManagedRole={id:string;name:string;enabled:boolean;actions:Action[]}
export type ManagedMember={user_id:string;name:string;enabled:boolean;kind:'employee'|'warehouse'|'supplier';roles:string[]}
export type AuthorizationDirectory={revision:string;actions:Action[];roles:ManagedRole[];members:ManagedMember[]}
export type AuthorizationCommand={operation_id:string;expected_revision:string;kind:'role.save';role:ManagedRole;user_id:null;roles:null}|{operation_id:string;expected_revision:string;kind:'member.roles';role:null;user_id:string;roles:string[]}
export async function getAuthorization(signal?:AbortSignal):Promise<AuthorizationDirectory>{
 const d=await request<AuthorizationDirectory>('authorization',{signal},'/api/admin/')
 const strings=(v:unknown):v is string[]=>Array.isArray(v)&&v.every(s=>typeof s==='string')&&new Set(v).size===v.length
 if(!d||typeof d.revision!=='string'||!/^[1-9]\d{0,19}$/.test(d.revision)||!strings(d.actions)||d.actions.length!==ACTIONS.length||d.actions.some(a=>!ACTIONS.includes(a))||!Array.isArray(d.roles)||d.roles.length>100||!Array.isArray(d.members)||d.members.length>1000)throw new APIError(0,'request.invalid_response')
 if(!d.roles.every(r=>r&&typeof r.id==='string'&&r.id.length>0&&typeof r.name==='string'&&typeof r.enabled==='boolean'&&strings(r.actions)&&r.actions.length<=ACTIONS.length&&r.actions.every(a=>ACTIONS.includes(a)))||new Set(d.roles.map(r=>r.id)).size!==d.roles.length)throw new APIError(0,'request.invalid_response')
 const ids=new Set(d.roles.map(r=>r.id))
 if(!d.members.every(m=>m&&typeof m.user_id==='string'&&m.user_id.length>0&&typeof m.name==='string'&&typeof m.enabled==='boolean'&&['employee','warehouse','supplier'].includes(m.kind)&&strings(m.roles)&&m.roles.length<=100&&m.roles.every(id=>ids.has(id)))||new Set(d.members.map(m=>m.user_id)).size!==d.members.length)throw new APIError(0,'request.invalid_response')
 return {revision:d.revision,actions:[...d.actions],roles:d.roles.map(({id,name,enabled,actions})=>({id,name,enabled,actions:[...actions]})),members:d.members.map(({user_id,name,enabled,kind,roles})=>({user_id,name,enabled,kind,roles:[...roles]}))}
}
export async function saveAuthorization(body:AuthorizationCommand):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const r=await request<{revision:string;replayed:boolean}>('authorization/commands',{method:'POST',csrf,body},'/api/admin/')
 if(!r||typeof r.revision!=='string'||!/^[1-9]\d{0,19}$/.test(r.revision)||typeof r.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
}
export type Permissions={revision:string;scope:'none'|'employee'|'warehouse'|'supplier';actions:Action[]}
export async function getPermissions(signal?:AbortSignal):Promise<Permissions>{
 const d=await request<Permissions>('permissions',{signal})
 if(!d||typeof d.revision!=='string'||!/^[1-9]\d{0,19}$/.test(d.revision)||!['none','employee','warehouse','supplier'].includes(d.scope)||!Array.isArray(d.actions)||d.actions.length>ACTIONS.length||d.actions.some(a=>!ACTIONS.includes(a))||new Set(d.actions).size!==d.actions.length||d.scope!=='employee'&&d.actions.length!==0)throw new APIError(0,'request.invalid_response')
 return {revision:d.revision,scope:d.scope,actions:[...d.actions]}
}
