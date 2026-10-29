export type User = { id: string; email: string; name: string; role: string; enabled: boolean }

export const WAREHOUSE_USER_ACTIONS=['inventory.read','inventory.stock.write','order.fulfillment.read','warehouse.intake.read','warehouse.intake.write','warehouse.return.read','warehouse.return.write'] as const
export type WarehouseUser={id:string;name:string;email:string;enabled:boolean;mode:'selected'|'all';locations:string[];actions:(typeof WAREHOUSE_USER_ACTIONS)[number][]}
export type WarehouseUserDirectory={revision:string;users:WarehouseUser[];locations:{id:string;name:string}[]}
export type WarehouseUserCommand=Omit<WarehouseUser,'id'>&{id:string;operation_id:string;expected_revision:string;password:string}
export async function warehouseUsers(signal?:AbortSignal):Promise<WarehouseUserDirectory>{
 const d=await request<WarehouseUserDirectory>('warehouse-users',{signal},'/api/admin/')
 if(!d||!warehouseRevision(d.revision)||!Array.isArray(d.users)||d.users.length>1000||!Array.isArray(d.locations)||d.locations.length>1000)throw new APIError(0,'request.invalid_response')
 const unique=(a:unknown):a is string[]=>Array.isArray(a)&&a.every(x=>typeof x==='string')&&new Set(a).size===a.length
 if(!d.locations.every(l=>l&&warehouseID(l.id)&&typeof l.name==='string')||!unique(d.locations.map(l=>l.id))||!unique(d.users.map(u=>u?.id)))throw new APIError(0,'request.invalid_response')
 if(!d.users.every(u=>u&&/^usr_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(u.id)&&typeof u.name==='string'&&typeof u.email==='string'&&typeof u.enabled==='boolean'&&['selected','all'].includes(u.mode)&&unique(u.locations)&&u.locations.length<=1000&&u.locations.every(warehouseID)&&(u.mode!=='all'||u.locations.length===0)&&unique(u.actions)&&u.actions.length<=WAREHOUSE_USER_ACTIONS.length&&u.actions.every(a=>WAREHOUSE_USER_ACTIONS.includes(a))))throw new APIError(0,'request.invalid_response')
 return {revision:d.revision,locations:d.locations.map(({id,name})=>({id,name})),users:d.users.map(({id,name,email,enabled,mode,locations,actions})=>({id,name,email,enabled,mode,locations:[...locations],actions:[...actions]}))}
}
export async function saveWarehouseUser(body:WarehouseUserCommand):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{id:string;revision:string;replayed:boolean}>('warehouse-users',{method:'POST',csrf,body},'/api/admin/')
 if(!d||!/^usr_[0-9a-f-]{36}$/.test(d.id)||!warehouseRevision(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
}

export class APIError extends Error {
  constructor(readonly status: number, readonly code: string, readonly requestID = '') {
    super(code)
  }
}

export type WarehouseAddress={address_1:string;address_2:string;company:string;city:string;country_code:string;phone:string;province:string;postal_code:string}
export type Warehouse={id:string;name:string;address:WarehouseAddress|null}
export type WarehouseDirectory={revision:string;locations:Warehouse[]}
export type WarehouseCommand={operation_id:string;expected_revision:string;id:string;name:string;address:WarehouseAddress}
const warehouseRevision=(v:unknown):v is string=>typeof v==='string'&&!/[\r\n]/.test(v)&&/^[1-9]\d{0,19}$/.test(v)&&BigInt(v)<=18446744073709551615n
const warehouseID=(v:unknown):v is string=>typeof v==='string'&&v.length>0&&Array.from(v).length<=191&&!/[\x00-\x1f\x7f/\\?#]/.test(v)
export async function warehouses(signal?:AbortSignal):Promise<WarehouseDirectory>{
 const d=await request<WarehouseDirectory>('warehouses',{signal},'/api/admin/')
 if(!d||!warehouseRevision(d.revision)||!Array.isArray(d.locations)||d.locations.length>1000)throw new APIError(0,'request.invalid_response')
 const locations=d.locations.map(row=>{
  if(!row||!warehouseID(row.id)||typeof row.name!=='string'||row.address!==null&&(!row.address||typeof row.address!=='object'||Object.keys(row.address).length!==8||['address_1','address_2','company','city','country_code','phone','province','postal_code'].some(k=>typeof row.address![k as keyof WarehouseAddress]!=='string')))throw new APIError(0,'request.invalid_response')
  return {id:row.id,name:row.name,address:row.address}
 })
 if(new Set(locations.map(row=>row.id)).size!==locations.length)throw new APIError(0,'request.invalid_response')
 return {revision:d.revision,locations}
}
export async function saveWarehouse(body:WarehouseCommand):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{id:string;revision:string;replayed:boolean}>('warehouses',{method:'POST',body,csrf},'/api/admin/')
 if(!d||!warehouseID(d.id)||!warehouseRevision(d.revision)||typeof d.replayed!=='boolean'||body.id!==''&&body.id!==d.id)throw new APIError(0,'request.invalid_response')
}

export type Mailbox={id:string;name:string;email:string;reply_to:string;enabled:boolean;has_api_key:boolean;has_webhook_secret:boolean;registration:boolean}
export type MailboxDirectory={revision:string;registration_account_id:string|null;accounts:Mailbox[]}
export type MailboxCommand={operation_id:string;expected_revision:string;id:string;name:string;email:string;reply_to:string;enabled:boolean;registration:boolean;api_key:string;webhook_secret:string}
const mailboxRevision=(v:unknown):v is string=>typeof v==='string'&&/^[1-9]\d{0,19}$/.test(v)&&BigInt(v)<=18446744073709551615n
const mailboxID=(v:unknown):v is string=>typeof v==='string'&&/^mail_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(v)
export async function mailboxes(signal?:AbortSignal):Promise<MailboxDirectory>{
 const d=await request<MailboxDirectory>('notification-mail/accounts',{signal},'/api/admin/')
 if(!d||!mailboxRevision(d.revision)||!Array.isArray(d.accounts)||d.accounts.length>100||d.registration_account_id!==null&&!mailboxID(d.registration_account_id))throw new APIError(0,'request.invalid_response')
 const accounts=d.accounts.map(a=>{
  if(!a||!mailboxID(a.id)||typeof a.name!=='string'||typeof a.email!=='string'||typeof a.reply_to!=='string'||typeof a.enabled!=='boolean'||typeof a.has_api_key!=='boolean'||typeof a.has_webhook_secret!=='boolean'||typeof a.registration!=='boolean'||a.registration!==(a.id===d.registration_account_id))throw new APIError(0,'request.invalid_response')
  return {id:a.id,name:a.name,email:a.email,reply_to:a.reply_to,enabled:a.enabled,has_api_key:a.has_api_key,has_webhook_secret:a.has_webhook_secret,registration:a.registration}
 })
 if(new Set(accounts.map(a=>a.id)).size!==accounts.length||d.registration_account_id!==null&&!accounts.some(a=>a.id===d.registration_account_id))throw new APIError(0,'request.invalid_response')
 return {revision:d.revision,registration_account_id:d.registration_account_id,accounts}
}
export async function saveMailbox(body:MailboxCommand):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{id:string;revision:string;replayed:boolean}>('notification-mail/accounts',{method:'POST',body,csrf},'/api/admin/')
 if(!d||!mailboxID(d.id)||!mailboxRevision(d.revision)||typeof d.replayed!=='boolean'||body.id!==''&&d.id!==body.id)throw new APIError(0,'request.invalid_response')
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
export type WarehouseAccess={revision:string;actions:(typeof WAREHOUSE_USER_ACTIONS)[number][];locations:{id:string;name:string}[]}
export type WarehouseInventoryLevel=Pick<InventoryLevel,'revision'|'id'|'location_id'|'stocked_quantity'|'reserved_quantity'|'incoming_quantity'|'available_quantity'>&{inventory_item_id:string;sku:string|null;title:string|null}
export type WarehouseInventoryPage={location_id:string;levels:WarehouseInventoryLevel[];offset:number;limit:50;has_more:boolean;writes_enabled:boolean}
export async function getWarehouseAccess(signal?:AbortSignal):Promise<WarehouseAccess>{
 const d=await request<WarehouseAccess>('warehouse/access',{signal},'/api/admin/')
 if(!d||!warehouseRevision(d.revision)||!Array.isArray(d.actions)||d.actions.length>WAREHOUSE_USER_ACTIONS.length||d.actions.some(a=>!WAREHOUSE_USER_ACTIONS.includes(a))||new Set(d.actions).size!==d.actions.length||!Array.isArray(d.locations)||d.locations.length>1000||!d.locations.every(l=>l&&warehouseID(l.id)&&typeof l.name==='string')||new Set(d.locations.map(l=>l.id)).size!==d.locations.length)throw new APIError(0,'request.invalid_response')
 return {revision:d.revision,actions:[...d.actions],locations:d.locations.map(({id,name})=>({id,name}))}
}
export async function getWarehouseInventory(location:string,offset:number,signal?:AbortSignal):Promise<WarehouseInventoryPage>{
 const d=await request<WarehouseInventoryPage>(`warehouse/locations/${encodeURIComponent(location)}/inventory?offset=${offset}`,{signal},'/api/admin/')
 if(!d||d.location_id!==location||d.offset!==offset||d.limit!==50||typeof d.has_more!=='boolean'||typeof d.writes_enabled!=='boolean'||!Array.isArray(d.levels)||d.levels.length>50||!d.levels.every(l=>l&&warehouseID(l.id)&&warehouseID(l.inventory_item_id)&&l.location_id===location&&nullableText(l.sku)&&nullableText(l.title)&&[l.stocked_quantity,l.reserved_quantity,l.incoming_quantity,l.available_quantity].every(inventoryDecimal)&&/^[a-f0-9]{64}$/.test(l.revision))||new Set(d.levels.map(l=>l.id)).size!==d.levels.length)throw new APIError(0,'request.invalid_response')
 return {...d,levels:d.levels.map(({id,inventory_item_id,location_id,sku,title,revision,stocked_quantity,reserved_quantity,incoming_quantity,available_quantity})=>({id,inventory_item_id,location_id,sku,title,revision,stocked_quantity,reserved_quantity,incoming_quantity,available_quantity}))}
}
export async function editWarehouseStock(location:string,id:string,body:StockEdit):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{revision:string;replayed:boolean}>(`warehouse/locations/${encodeURIComponent(location)}/inventory/${encodeURIComponent(id)}/stock`,{method:'POST',body,csrf},'/api/admin/')
 if(!d||typeof d.revision!=='string'||!/^[a-f0-9]{64}$/.test(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
}
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


export const ACTIONS=['authorization.read','authorization.write','device.enroll','device.intake','device.read','device.source.write','inventory.read','inventory.stock.write','mail.read','mail.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','price.write','product.read','product.write','warehouse.intake.read','warehouse.intake.write','warehouse.read','warehouse.return.read','warehouse.return.write','warehouse.write'] as const
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

export type OrderMailJob={id:string;order_id:string;status:'pending'|'sending'|'sent'|'failed';attempts:number;created_at:string;first_attempt_at:string|null;next_attempt_at:string|null;sent_at:string|null;failed_at:string|null;error_code:string|null;retryable:boolean}
export type OrderMailJobs={jobs:OrderMailJob[];count:number;limit:number;offset:number}
export type OrderMailRetry={operation_id:string;expected_attempts:number}
const mailJobID=(v:unknown):v is string=>typeof v==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(v)
export async function orderMailJobs(offset:number,signal?:AbortSignal):Promise<OrderMailJobs>{
 if(!Number.isSafeInteger(offset)||offset<0||offset>100000)throw new APIError(400,'request.invalid_input')
 const d=await request<OrderMailJobs>(`notification-mail/order-jobs?offset=${offset}`,{signal},'/api/admin/')
 if(!d||!Array.isArray(d.jobs)||d.jobs.length>50||d.limit!==50||d.offset!==offset||!Number.isSafeInteger(d.count)||d.count<0)throw new APIError(0,'request.invalid_response')
 const dates=['first_attempt_at','next_attempt_at','sent_at','failed_at'] as const
 for(const j of d.jobs){
  if(!j||!mailJobID(j.id)||typeof j.order_id!=='string'||!/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(j.order_id)||!['pending','sending','sent','failed'].includes(j.status)||!Number.isSafeInteger(j.attempts)||j.attempts<0||typeof j.created_at!=='string'||!Number.isFinite(Date.parse(j.created_at))||typeof j.retryable!=='boolean'||j.retryable&&j.status!=='failed'||j.error_code!==null&&(typeof j.error_code!=='string'||! /^[a-z_]{1,32}$/.test(j.error_code)))throw new APIError(0,'request.invalid_response')
  for(const key of dates)if(j[key]!==null&&(typeof j[key]!=='string'||!Number.isFinite(Date.parse(j[key]!))))throw new APIError(0,'request.invalid_response')
 }
 if(new Set(d.jobs.map(j=>j.id)).size!==d.jobs.length)throw new APIError(0,'request.invalid_response')
 return d
}
export async function retryOrderMail(id:string,body:OrderMailRetry):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 if(!mailJobID(id))throw new APIError(400,'request.invalid_input')
 const d=await request<{id:string;replayed:boolean}>(`notification-mail/order-jobs/${encodeURIComponent(id)}/retry`,{method:'POST',body,csrf},'/api/admin/')
 if(!d||d.id!==id||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
}

export type DeviceTrackingState={product_id:string;variant_id:string;revision:string;enabled:boolean;eligible:boolean;reason:''|'already_enabled'|'inventory_configuration'|'shared_inventory_item';inventory_item_id:string}
export type DeviceEnrollInput={operation_id:string;expected_revision:string}
function checkTracking(d:DeviceTrackingState,productID:string,variantID:string):DeviceTrackingState{
 if(!d||d.product_id!==productID||d.variant_id!==variantID||typeof d.revision!=='string'||!/^[0-9a-f]{64}$/.test(d.revision)||typeof d.enabled!=='boolean'||typeof d.eligible!=='boolean'||typeof d.inventory_item_id!=='string'||d.inventory_item_id!==''&&!warehouseID(d.inventory_item_id)||!['','already_enabled','inventory_configuration','shared_inventory_item'].includes(d.reason)||d.enabled&&(d.eligible||d.reason!=='already_enabled')||d.eligible&&d.reason!=='')throw new APIError(0,'request.invalid_response')
 return {product_id:d.product_id,variant_id:d.variant_id,revision:d.revision,enabled:d.enabled,eligible:d.eligible,reason:d.reason,inventory_item_id:d.inventory_item_id}
}
export async function deviceTracking(productID:string,variantID:string,signal?:AbortSignal):Promise<DeviceTrackingState>{
 const d=await request<DeviceTrackingState>(`products/${encodeURIComponent(productID)}/variants/${encodeURIComponent(variantID)}/device-tracking`,{signal},'/api/admin/');return checkTracking(d,productID,variantID)
}
export async function enrollDeviceTracking(productID:string,variantID:string,body:DeviceEnrollInput):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{tracking:DeviceTrackingState;replayed:boolean}>(`products/${encodeURIComponent(productID)}/variants/${encodeURIComponent(variantID)}/device-tracking`,{method:'POST',body,csrf},'/api/admin/')
 if(!d||typeof d.replayed!=='boolean'||!checkTracking(d.tracking,productID,variantID).enabled)throw new APIError(0,'request.invalid_response')
}

export type DeviceSource={id:string;name:string;type:'self'|'supplier';location_id:string;location_name:string|null}
export type DeviceSourcesDirectory={revision:string;sources:DeviceSource[]}
export type DeviceSourceInput={operation_id:string;expected_revision:string;name:string;type:'self'|'supplier'}
function sourceProjection(s:DeviceSource):DeviceSource{
 if(!s||!warehouseID(s.id)||!warehouseID(s.location_id)||typeof s.name!=='string'||!['self','supplier'].includes(s.type)||s.location_name!==null&&typeof s.location_name!=='string')throw new APIError(0,'request.invalid_response')
 return {id:s.id,name:s.name,type:s.type,location_id:s.location_id,location_name:s.location_name}
}
export async function deviceSources(signal?:AbortSignal):Promise<DeviceSourcesDirectory>{
 const d=await request<DeviceSourcesDirectory>('device-sources',{signal},'/api/admin/')
 if(!d||!warehouseRevision(d.revision)||!Array.isArray(d.sources)||d.sources.length>1000)throw new APIError(0,'request.invalid_response')
 const sources=d.sources.map(sourceProjection);if(new Set(sources.map(s=>s.id)).size!==sources.length)throw new APIError(0,'request.invalid_response');return {revision:d.revision,sources}
}
export async function createDeviceSource(body:DeviceSourceInput):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{source:DeviceSource;revision:string;replayed:boolean}>('device-sources',{method:'POST',body,csrf},'/api/admin/')
 if(!d||!warehouseRevision(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
 const source=sourceProjection(d.source);if(source.name!==body.name||source.type!==body.type||source.location_name===null)throw new APIError(0,'request.invalid_response')
}

export type DeviceIntakeRow={serial_number:string;imei:string;imei2:string;eid:string;source_id:string;category:'sale'|'returned'|'recycled';grade:'S'|'A'|'B'|'C'|null;battery_health:number|null;cost_cents:number;currency_code:string;bin:string;notes:string;photos:{id:string;url:string}[]}
export type RecentDevice={id:string;serial_number:string;imei:string|null;imei2:string|null;eid:string|null;category:'sale'|'returned'|'recycled';state:string;location_id:string;grade:string|null;battery_health:number|null;created_at:string;photos:DevicePhoto[]}
export type DeviceIntakeContext={recent_devices:RecentDevice[];recent_limit:50;revision:string;requires_radio:boolean;options:{title:string;value:string}[];sources:DeviceSource[]}
export type DeviceIntakeInput={operation_id:string;expected_revision:string;devices:DeviceIntakeRow[]}
export type DeviceIntakeRecord=Omit<DeviceIntakeRow,'grade'>&{grade:string|null;id:string;variant_id:string;location_id:string;state:'available'|'inspection'}
export async function deviceIntakeContext(productID:string,variantID:string,signal?:AbortSignal,scope?:WarehouseIntakeScope):Promise<DeviceIntakeContext>{
 const d=await request<DeviceIntakeContext>(intakePath(productID,variantID,scope),{signal},'/api/admin/')
 if(!d||!/^[0-9a-f]{64}$/.test(d.revision)||typeof d.requires_radio!=='boolean'||!Array.isArray(d.options)||d.options.length>100||d.options.some(o=>!o||typeof o.title!=='string'||typeof o.value!=='string')||!Array.isArray(d.sources)||d.sources.length>1000)throw new APIError(0,'request.invalid_response')
 if(d.recent_limit!==50||!Array.isArray(d.recent_devices)||d.recent_devices.length>50||new Set(d.recent_devices.map(r=>r.id)).size!==d.recent_devices.length||d.recent_devices.some(r=>!r||!warehouseID(r.id)||typeof r.serial_number!=='string'||!warehouseID(r.location_id)||!['sale','returned','recycled'].includes(r.category)||!['pending','inspection','available','reserved','packed','shipped','repair','repair_ready','retired','supplier_returned'].includes(r.state)||typeof r.created_at!=='string'||!Number.isFinite(Date.parse(r.created_at))||[r.imei,r.imei2,r.eid].some(v=>v!==null&&typeof v!=='string')||!Array.isArray(r.photos)||r.photos.length>8||r.photos.some(p=>!validDevicePhoto(p))))throw new APIError(0,'request.invalid_response')
 if(scope&&(d.sources.some(s=>s.location_id!==scope.locationID)||d.recent_devices.some(r=>r.location_id!==scope.locationID||r.category!==scope.category)))throw new APIError(0,'request.invalid_response')
 const sources=d.sources.map(sourceProjection);if(sources.some(s=>s.type!=='self'||s.location_name===null)||new Set(sources.map(s=>s.id)).size!==sources.length)throw new APIError(0,'request.invalid_response')
 return {recent_devices:d.recent_devices.map(r=>({id:r.id,serial_number:r.serial_number,imei:r.imei,imei2:r.imei2,eid:r.eid,category:r.category,state:r.state,location_id:r.location_id,grade:r.grade,battery_health:r.battery_health,created_at:r.created_at,photos:r.photos.map(p=>({id:p.id,url:p.url}))})),recent_limit:50,revision:d.revision,requires_radio:d.requires_radio,options:d.options.map(o=>({title:o.title,value:o.value})),sources}
}
export async function intakeDevices(productID:string,variantID:string,body:DeviceIntakeInput,scope?:WarehouseIntakeScope):Promise<DeviceIntakeRecord[]>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{devices:DeviceIntakeRecord[];replayed:boolean}>(intakePath(productID,variantID,scope),{method:'POST',body,csrf},'/api/admin/')
 if(!d||typeof d.replayed!=='boolean'||!Array.isArray(d.devices)||d.devices.length!==body.devices.length||new Set(d.devices.map(r=>r.id)).size!==d.devices.length)throw new APIError(0,'request.invalid_response')
 for(const [i,r] of d.devices.entries()){const input=body.devices[i];if(!r||!warehouseID(r.id)||r.variant_id!==variantID||!warehouseID(r.location_id)||r.serial_number!==input.serial_number.trim().toUpperCase()||r.source_id!==input.source_id||r.category!==input.category||r.state!==(input.category==='sale'?'available':'inspection')||r.imei!==input.imei||r.imei2!==input.imei2||r.eid!==input.eid||r.cost_cents!==input.cost_cents||r.currency_code!==input.currency_code||r.grade!==null&&typeof r.grade!=='string'||r.battery_health!==null&&(!Number.isInteger(r.battery_health)||r.battery_health<0||r.battery_health>100)||!Array.isArray(r.photos)||r.photos.length!==input.photos.length||r.photos.some((p,n)=>!validDevicePhoto(p)||p.id!==input.photos[n].id||p.url!==input.photos[n].url))throw new APIError(0,'request.invalid_response')}
 return d.devices
}

export type DevicePhoto={id:string;url:string}
const validDevicePhoto=(p:DevicePhoto)=>!!p&&/^dphoto_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(p.id)&&p.url===`/api/store/device-photos/${p.id}`
export async function uploadDevicePhoto(body:{operation_id:string;filename:string;mime_type:string;content:string},scope?:WarehouseIntakeScope):Promise<DevicePhoto>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{photo:DevicePhoto;replayed:boolean}>(scope?warehouseIntakeBase(scope)+'/photos':'device-photos',{method:'POST',body,csrf},'/api/admin/')
 if(!d||typeof d.replayed!=='boolean'||!validDevicePhoto(d.photo))throw new APIError(0,'request.invalid_response')
 return {id:d.photo.id,url:d.photo.url}
}

export type WarehouseDeviceAction='activate'|'make_available'|'retire'|'delete'|'transfer_return'
export type WarehouseDevice=RecentDevice&{variant_id:string;variant_title:string|null;source_id:string;bin:string;revision:string;allowed_actions:WarehouseDeviceAction[]}
export type WarehouseDevicePage={location_id:string;category:'sale'|'returned';devices:WarehouseDevice[];offset:number;limit:50;has_more:boolean}
export type WarehouseDeviceCommand={operation_id:string;expected_revision:string;action:WarehouseDeviceAction;notes?:string}
export async function getWarehouseDevices(location:string,category:'sale'|'returned',offset:number,q:string,signal?:AbortSignal):Promise<WarehouseDevicePage>{
 const d=await request<WarehouseDevicePage>(`warehouse/locations/${encodeURIComponent(location)}/devices/${category}?offset=${offset}&q=${encodeURIComponent(q)}`,{signal},'/api/admin/')
 if(!d||d.location_id!==location||d.category!==category||d.offset!==offset||d.limit!==50||typeof d.has_more!=='boolean'||!Array.isArray(d.devices)||d.devices.length>50||new Set(d.devices.map(r=>r.id)).size!==d.devices.length)throw new APIError(0,'request.invalid_response')
 for(const r of d.devices)if(!r||!warehouseID(r.id)||r.location_id!==location||r.category!==category||!warehouseID(r.variant_id)||!warehouseID(r.source_id)||typeof r.serial_number!=='string'||typeof r.bin!=='string'||r.variant_title!==null&&typeof r.variant_title!=='string'||!/^[a-f0-9]{64}$/.test(r.revision)||!Array.isArray(r.allowed_actions)||new Set(r.allowed_actions).size!==r.allowed_actions.length||r.allowed_actions.some(a=>!['activate','make_available','retire','delete','transfer_return'].includes(a))||!Array.isArray(r.photos)||r.photos.length>8||r.photos.some(p=>!validDevicePhoto(p))||[r.imei,r.imei2,r.eid].some(v=>v!==null&&typeof v!=='string'))throw new APIError(0,'request.invalid_response')
 return d
}
export async function commandWarehouseDevice(location:string,category:'sale'|'returned',id:string,body:WarehouseDeviceCommand):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{id:string;state:string;deleted:boolean;replayed:boolean}>(`warehouse/locations/${encodeURIComponent(location)}/devices/${category}/${encodeURIComponent(id)}`,{method:'POST',body,csrf},'/api/admin/')
 if(!d||d.id!==id||typeof d.replayed!=='boolean'||d.deleted!==(body.action==='delete')||typeof d.state!=='string'||body.action==='retire'&&d.state!=='retired'||body.action==='transfer_return'&&d.state!=='inspection'||['activate','make_available'].includes(body.action)&&d.state!=='available')throw new APIError(0,'request.invalid_response')
}

export type WarehouseIntakeScope={locationID:string;category:'sale'|'returned'}
function warehouseIntakeBase(scope:WarehouseIntakeScope){return `warehouse/locations/${encodeURIComponent(scope.locationID)}/device-intake/${scope.category}`}
function intakePath(productID:string,variantID:string,scope?:WarehouseIntakeScope){return scope?`${warehouseIntakeBase(scope)}/products/${encodeURIComponent(productID)}/variants/${encodeURIComponent(variantID)}`:`products/${encodeURIComponent(productID)}/variants/${encodeURIComponent(variantID)}/device-intake`}
export type WarehouseIntakeVariant={id:string;product_id:string;title:string;product_title:string;sku:string|null}
export async function warehouseIntakeVariants(scope:WarehouseIntakeScope,q:string,offset:number,signal?:AbortSignal){
 const d=await request<{variants:WarehouseIntakeVariant[];offset:number;limit:20;has_more:boolean}>(`${warehouseIntakeBase(scope)}/variants?q=${encodeURIComponent(q)}&offset=${offset}`,{signal},'/api/admin/')
 if(!d||d.offset!==offset||d.limit!==20||typeof d.has_more!=='boolean'||!Array.isArray(d.variants)||d.variants.length>20||new Set(d.variants.map(v=>v.id)).size!==d.variants.length||d.variants.some(v=>!v||!warehouseID(v.id)||!warehouseID(v.product_id)||typeof v.title!=='string'||typeof v.product_title!=='string'||v.sku!==null&&typeof v.sku!=='string'))throw new APIError(0,'request.invalid_response')
 return d
}

export type PriceAmountEdit={operation_id:string;expected_revision:string;amount:string}
export type PriceEditContext={effective_amount:string;product_id:string;variant_id:string;id:string;price_set_id:string;currency_code:string;amount:string;min_quantity:string|null;max_quantity:string|null;price_list_id:string|null;shared_variants:number;enabled:boolean;revision:string}
const priceEditPath=(product:string,variant:string,price:string)=>`products/${encodeURIComponent(product)}/variants/${encodeURIComponent(variant)}/prices/${encodeURIComponent(price)}`
export async function readPriceEdit(product:string,variant:string,price:string,signal:AbortSignal):Promise<PriceEditContext>{
 const d=await request<PriceEditContext>(priceEditPath(product,variant,price),{signal},'/api/admin/')
 if(!d||d.product_id!==product||d.variant_id!==variant||d.id!==price||typeof d.effective_amount!=='string'||d.effective_amount.length>1024||!/^(0|[1-9]\d*)(\.\d+)?$/.test(d.effective_amount)||typeof d.price_set_id!=='string'||typeof d.currency_code!=='string'||!decimalText(d.amount)||!nullableDecimal(d.min_quantity)||!nullableDecimal(d.max_quantity)||!nullableText(d.price_list_id)||!Number.isSafeInteger(d.shared_variants)||d.shared_variants<1||typeof d.enabled!=='boolean'||typeof d.revision!=='string'||!/^[a-f0-9]{64}$/.test(d.revision))throw new APIError(0,'request.invalid_response')
 return d
}
export async function editPriceAmount(product:string,variant:string,price:string,body:PriceAmountEdit):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{revision:string;replayed:boolean}>(priceEditPath(product,variant,price),{method:'POST',body,csrf},'/api/admin/')
 if(!d||typeof d.revision!=='string'||!/^[a-f0-9]{64}$/.test(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
}

export type PriceCreateContext={product_id:string;variant_id:string;price_set_id:string|null;currencies:string[];existing_base_currencies:string[];shared_variants:number;enabled:boolean;revision:string}
export type PriceCreateInput=PriceAmountEdit&{currency_code:string}
export async function readPriceCreate(product:string,variant:string,signal:AbortSignal):Promise<PriceCreateContext>{
 const d=await request<PriceCreateContext>(`products/${encodeURIComponent(product)}/variants/${encodeURIComponent(variant)}/price-creation`,{signal},'/api/admin/')
 const codes=(v:unknown):v is string[]=>Array.isArray(v)&&v.length<=1000&&v.every(c=>typeof c==='string'&&/^[a-z]{3}$/.test(c))&&new Set(v).size===v.length
 if(!d||d.product_id!==product||d.variant_id!==variant||!nullableText(d.price_set_id)||!codes(d.currencies)||!codes(d.existing_base_currencies)||!Number.isSafeInteger(d.shared_variants)||d.shared_variants<0||typeof d.enabled!=='boolean'||typeof d.revision!=='string'||!/^[a-f0-9]{64}$/.test(d.revision))throw new APIError(0,'request.invalid_response')
 return d
}
export async function createBasePrice(product:string,variant:string,body:PriceCreateInput):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{id:string;revision:string;replayed:boolean}>(`products/${encodeURIComponent(product)}/variants/${encodeURIComponent(variant)}/prices`,{method:'POST',body,csrf},'/api/admin/')
 if(!d||typeof d.id!=='string'||!d.id.startsWith('price_')||typeof d.revision!=='string'||!/^[a-f0-9]{64}$/.test(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
}

export type VariantCreateContext={product_id:string;options:ProductOption[];enabled:boolean;revision:string}
export type VariantCreateInput={operation_id:string;expected_revision:string;title:string;sku:string;option_value_ids:string[]}
export async function readVariantCreate(product:string,signal:AbortSignal):Promise<VariantCreateContext>{
 const d=await request<VariantCreateContext>(`products/${encodeURIComponent(product)}/variant-creation`,{signal},'/api/admin/')
 if(!d||d.product_id!==product||typeof d.enabled!=='boolean'||typeof d.revision!=='string'||!/^[a-f0-9]{64}$/.test(d.revision)||!Array.isArray(d.options)||d.options.length>1000||d.options.some(o=>!o||typeof o.id!=='string'||typeof o.title!=='string'||!Array.isArray(o.values)||o.values.length>10000||o.values.some(v=>!v||typeof v.id!=='string'||typeof v.value!=='string')))throw new APIError(0,'request.invalid_response')
 return d
}
export async function createVariant(product:string,body:VariantCreateInput):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{id:string;inventory_item_id:string;sku:string;revision:string;replayed:boolean}>(`products/${encodeURIComponent(product)}/variants`,{method:'POST',body,csrf},'/api/admin/')
 if(!d||typeof d.id!=='string'||!d.id.startsWith('variant_')||typeof d.inventory_item_id!=='string'||!d.inventory_item_id.startsWith('iitem_')||typeof d.sku!=='string'||!d.sku||typeof d.revision!=='string'||!/^[a-f0-9]{64}$/.test(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
}

export type CreateLevelContext={product_id:string;variant_id:string;inventory_item_id:string;locations:{id:string;name:string;has_level:boolean}[];shared_variants:number;enabled:boolean;revision:string}
export type CreateLevelInput=StockEdit&{location_id:string}
const levelCreatePath=(p:string,v:string,i:string)=>`products/${encodeURIComponent(p)}/variants/${encodeURIComponent(v)}/inventory/${encodeURIComponent(i)}`
export async function readCreateLevel(product:string,variant:string,item:string,signal:AbortSignal):Promise<CreateLevelContext>{
 const d=await request<CreateLevelContext>(levelCreatePath(product,variant,item)+'/level-creation',{signal},'/api/admin/')
 if(!d||d.product_id!==product||d.variant_id!==variant||d.inventory_item_id!==item||typeof d.enabled!=='boolean'||typeof d.revision!=='string'||!/^[a-f0-9]{64}$/.test(d.revision)||!Number.isSafeInteger(d.shared_variants)||d.shared_variants<1||!Array.isArray(d.locations)||d.locations.length>1000||d.locations.some(l=>!l||typeof l.id!=='string'||typeof l.name!=='string'||typeof l.has_level!=='boolean')||new Set(d.locations.map(l=>l.id)).size!==d.locations.length)throw new APIError(0,'request.invalid_response')
 return d
}
export async function createInventoryLevel(product:string,variant:string,item:string,body:CreateLevelInput):Promise<void>{
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{id:string;revision:string;replayed:boolean}>(levelCreatePath(product,variant,item)+'/levels',{method:'POST',body,csrf},'/api/admin/')
 if(!d||typeof d.id!=='string'||!d.id.startsWith('ilevel_')||typeof d.revision!=='string'||!/^[a-f0-9]{64}$/.test(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
}
